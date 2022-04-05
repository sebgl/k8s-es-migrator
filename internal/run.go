package internal

import (
	"context"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Config struct {
	FromContext string
	ToContext   string
	Namespace   string
	Name        string
}

func Run(cfg Config) error {
	clientFrom, err := newK8sClient(cfg.FromContext)
	if err != nil {
		return err
	}
	clientTo, err := newK8sClient(cfg.ToContext)
	if err != nil {
		return err
	}
	migrator := migrator{
		clientFrom: clientFrom,
		clientTo:   clientTo,
		namespace:  cfg.Namespace,
		name:       cfg.Name,
	}
	return migrator.Migrate()
}

func newK8sClient(kubeContext string) (client.Client, error) {
	clientCfg, err := config.GetConfigWithContext(kubeContext)
	if err != nil {
		return nil, err
	}
	// speed things up a bit
	clientCfg.Burst = 100
	clientCfg.QPS = 100
	c, err := client.New(clientCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	// check the client can connect by listing ES resources
	var esList esv1.ElasticsearchList
	if err := c.List(context.Background(), &esList); err != nil {
		return nil, err
	}
	return c, nil
}
