package internal

import (
	"context"
	"errors"
	"fmt"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/bootstrap"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var startTime time.Time

func init() {
	startTime = time.Now()
}

func logger() *log.Entry {
	return log.WithField("timestamp", time.Since(startTime).Round(time.Second))
}

type migrator struct {
	clientFrom client.Client
	clientTo   client.Client

	namespace string
	name      string
}

func (m migrator) Migrate() error {
	resourcesFrom, err := m.retrieveExistingResources()
	if err != nil {
		return err
	}
	if err := setPVsReclaimPolicy(m.clientFrom, resourcesFrom.pvs); err != nil {
		return err
	}
	if err := deleteElasticsearch(m.clientFrom, resourcesFrom.elasticsearch); err != nil {
		return err
	}
	if err := recreateResources(m.clientTo, resourcesFrom); err != nil {
		return err
	}
	if err := checkPodsRunning(m.clientTo, resourcesFrom.pods); err != nil {
		return err
	}
	if err := checkNewClusterUUID(m.clientTo, resourcesFrom.elasticsearch); err != nil {
		return err
	}
	if err := deletePVs(m.clientFrom, resourcesFrom.pvs); err != nil {
		return err
	}
	logger().Info("Migration successful!")
	return nil
}

type resourcesFrom struct {
	elasticsearch esv1.Elasticsearch
	pods          []corev1.Pod
	pvcs          []corev1.PersistentVolumeClaim
	pvs           []corev1.PersistentVolume
}

func (m migrator) retrieveExistingResources() (resourcesFrom, error) {
	// retrieve ES resource
	logger().WithField("namespace", m.namespace).WithField("name", m.name).Info("Retrieving Elasticsearch in source K8s cluster")
	var es esv1.Elasticsearch
	if err := m.clientFrom.Get(context.Background(), types.NamespacedName{Namespace: m.namespace, Name: m.name}, &es); err != nil {
		return resourcesFrom{}, err
	}

	// retrieve Pods
	logger().WithField("namespace", m.namespace).WithField("name", m.name).Info("Retrieving Pods in source K8s cluster")
	var pods corev1.PodList
	if err := m.clientFrom.List(context.Background(), &pods, client.InNamespace(m.namespace), client.MatchingLabels{
		"elasticsearch.k8s.elastic.co/cluster-name": m.name,
	}); err != nil {
		return resourcesFrom{}, err
	}

	// retrieve labeled PVCs
	logger().WithField("namespace", m.namespace).WithField("name", m.name).Info("Retrieving PVCs in source K8s cluster")
	var pvcs corev1.PersistentVolumeClaimList
	if err := m.clientFrom.List(
		context.Background(),
		&pvcs,
		client.InNamespace(m.namespace),
		client.MatchingLabels{
			"elasticsearch.k8s.elastic.co/cluster-name": m.name,
		},
	); err != nil {
		return resourcesFrom{}, err
	}
	if len(pvcs.Items) == 0 {
		return resourcesFrom{}, errors.New("no PVC found")
	}

	// retrieve PVs matching PVCs
	var pvs []corev1.PersistentVolume
	for _, pvc := range pvcs.Items {
		volumeName := pvc.Spec.VolumeName
		if volumeName == "" {
			return resourcesFrom{}, fmt.Errorf("PVC %s/%s has no spec.volumeName", pvc.Namespace, pvc.Name)
		}
		logger().WithField("name", volumeName).Info("Retrieving PV in source K8s cluster")
		var pv corev1.PersistentVolume
		if err := m.clientFrom.Get(context.Background(), types.NamespacedName{Namespace: pvc.Namespace, Name: volumeName}, &pv); err != nil {
			return resourcesFrom{}, err
		}
		pvs = append(pvs, pv)
	}

	return resourcesFrom{
		elasticsearch: es,
		pods:          pods.Items,
		pvcs:          pvcs.Items,
		pvs:           pvs,
	}, nil
}

// setPVsReclaimPolicy ensures all pvs have their spec.retainPolicy set to 'Retain'.
func setPVsReclaimPolicy(client client.Client, pvs []corev1.PersistentVolume) error {
	for _, pv := range pvs {
		if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
			// nothing to do for that one
			continue
		}
		logger().WithField("name", pv.Name).Info("Setting spec.persistentVolumeClaimPolicy=Retain on PV in source K8s cluster")
		pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
		if err := client.Update(context.Background(), &pv); err != nil {
			return err
		}
	}
	return nil
}

func deleteElasticsearch(c client.Client, es esv1.Elasticsearch) error {
	logger().WithField("namespace", es.Namespace).WithField("name", es.Name).Info("Deleting ES resource in source K8s cluster")
	if err := c.Delete(context.Background(), &es, &client.DeleteOptions{}); err != nil {
		return err
	}

	// also force-delete all Pods to speed things up
	var pods corev1.PodList
	if err := c.List(context.Background(), &pods, client.InNamespace(es.Namespace), client.MatchingLabels{
		"elasticsearch.k8s.elastic.co/cluster-name": es.Name,
	}); err != nil {
		return err
	}
	zero := int64(0)
	for _, p := range pods.Items {
		logger().WithField("namespace", p.Namespace).WithField("name", p.Name).Info("Force-deleting Pod in source K8s cluster")
		if err := c.Delete(context.Background(), &p, &client.DeleteOptions{
			// immediate deletion ignoring the pre-stop hook
			GracePeriodSeconds: &zero,
		}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func recreateResources(c client.Client, resources resourcesFrom) error {
	// create PVs
	for _, pv := range resources.pvs {
		toCreate := *pv.DeepCopy()
		// zero-out some fields we don't want to propagate
		toCreate.CreationTimestamp = metav1.Time{}
		toCreate.ResourceVersion = ""
		toCreate.UID = ""
		if toCreate.Spec.ClaimRef == nil {
			return fmt.Errorf("spec.claimRef is nil on PV %s/%s", pv.Namespace, pv.Name)
		}
		toCreate.Spec.ClaimRef.UID = ""
		toCreate.Spec.ClaimRef.ResourceVersion = ""
		toCreate.Status = corev1.PersistentVolumeStatus{}

		logger().WithField("name", pv.Name).Info("Creating PV in target cluster (same backing CSP volume)")
		if err := c.Create(context.Background(), &toCreate); err != nil {
			return err
		}
	}

	// create ES
	toCreate := *resources.elasticsearch.DeepCopy()
	// zero-out some fields we don't want to propagate
	toCreate.CreationTimestamp = metav1.Time{}
	toCreate.ResourceVersion = ""
	toCreate.UID = ""
	toCreate.OwnerReferences = nil
	toCreate.Annotations = nil
	toCreate.Status = esv1.ElasticsearchStatus{}
	logger().WithField("namespace", toCreate.Namespace).WithField("name", toCreate.Name).Info("Creating Elasticsearch in target cluster")
	return c.Create(context.Background(), &toCreate)
}

func checkPodsRunning(c client.Client, previousPods []corev1.Pod) error {
	if len(previousPods) == 0 {
		return nil
	}
	logger().WithField("namespace", previousPods[0].Namespace).Info("Waiting for all volumes to be bound and Pods to be running in target cluster")
	retries := 30
	retryDelay := 5 * time.Second
	for i := 0; i < retries; i++ {
		ok, err := allPodsRunning(c, previousPods)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		// check again
		time.Sleep(retryDelay)
	}
	return errors.New("not all Pods are running")
}

func allPodsRunning(c client.Client, previousPods []corev1.Pod) (bool, error) {
	for _, p := range previousPods {
		var retrievedPod corev1.Pod
		err := c.Get(context.Background(), types.NamespacedName{Namespace: p.Namespace, Name: p.Name}, &retrievedPod)
		if apierrors.IsNotFound(err) {
			// Pod not created yet
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if retrievedPod.Status.Phase != corev1.PodRunning {
			// Pod not running yet
			return false, nil
		}
	}
	return true, nil
}

func checkNewClusterUUID(c client.Client, previousES esv1.Elasticsearch) error {
	logger().WithField("namespace", previousES.Namespace).WithField("name", previousES.Name).Info("Waiting for Elasticsearch UUID to be reported (previous UUID should be preserved)")
	retries := 30
	retryDelay := 5 * time.Second
	for i := 0; i < retries; i++ {
		var retrieved esv1.Elasticsearch
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: previousES.Namespace, Name: previousES.Name}, &retrieved); err != nil {
			return err
		}
		switch retrieved.Annotations[bootstrap.ClusterUUIDAnnotationName] {
		case "":
			time.Sleep(retryDelay)
			continue
		case previousES.Annotations[bootstrap.ClusterUUIDAnnotationName]:
			logger().WithField("namespace", retrieved.Namespace).WithField("name", retrieved.Name).WithField("UUID", retrieved.Annotations[bootstrap.ClusterUUIDAnnotationName]).Info("Cluster UUID successfully preserved!")
			return nil
		default:
			logger().
				WithField("namespace", retrieved.Namespace).WithField("name", retrieved.Name).
				WithField("UUID", retrieved.Annotations[bootstrap.ClusterUUIDAnnotationName]).
				WithField("expected_UUID", previousES.Annotations[bootstrap.ClusterUUIDAnnotationName]).
				Info("Unexpected: cluster UUID has changed!")
			return fmt.Errorf("expected UUID %s, got %s", previousES.Annotations[bootstrap.ClusterUUIDAnnotationName], retrieved.Annotations[bootstrap.ClusterUUIDAnnotationName])
		}
	}
	return fmt.Errorf("got no UUID after %d retries", retries)
}

func deletePVs(c client.Client, pvs []corev1.PersistentVolume) error {
	for _, pv := range pvs {
		logger().WithField("name", pv.Name).Info("Deleting PV in source cluster")
		if err := c.Delete(context.Background(), &pv); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
