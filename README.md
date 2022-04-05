# Elasticsearch migration between K8s clusters

## With downtime, in the same CSP region

This flow consists in:
- deleting Elasticsearch in cluster A, while making sure PersistentVolumes are retained
- recreating those PVs in cluster B
- recreating Elasticsearch in cluster B (reusing the same PVs)

Usage:

```
go run main.go <es-namespace>/<es-name> --from=<kubeconfig-context-A> --to=<kubeconfig-context-B>
```

This assumes Elasticsearch deployed in the first K8s cluster, and the 2 K8s clusters present in the same local kubeconfig file (2 different contexts).

Example:

```
❯ go run main.go default/elasticsearch-sample --from=gke_elastic-cloud-dev_europe-west1_sebgl-dev-cluster --to=gke_elastic-cloud-dev_europe-west1_sebgl-dev-cluster2
INFO[0000] Retrieving Elasticsearch in source K8s cluster  name=elasticsearch-sample namespace=default timestamp=0s
INFO[0000] Retrieving Pods in source K8s cluster         name=elasticsearch-sample namespace=default timestamp=0s
INFO[0000] Retrieving PVCs in source K8s cluster         name=elasticsearch-sample namespace=default timestamp=0s
INFO[0000] Retrieving PV in source K8s cluster           name=pvc-fd2aa71e-a0b9-4b0c-8437-f4dc27ba65c9 timestamp=1s
INFO[0000] Retrieving PV in source K8s cluster           name=pvc-9af782fd-27ed-42a3-b563-12dea569432f timestamp=1s
INFO[0000] Retrieving PV in source K8s cluster           name=pvc-b38e0b86-d884-4503-bb7d-d64b03e54ec5 timestamp=1s
INFO[0000] Setting spec.persistentVolumeClaimPolicy=Retain on PV in source K8s cluster  name=pvc-fd2aa71e-a0b9-4b0c-8437-f4dc27ba65c9 timestamp=1s
INFO[0000] Setting spec.persistentVolumeClaimPolicy=Retain on PV in source K8s cluster  name=pvc-9af782fd-27ed-42a3-b563-12dea569432f timestamp=1s
INFO[0000] Setting spec.persistentVolumeClaimPolicy=Retain on PV in source K8s cluster  name=pvc-b38e0b86-d884-4503-bb7d-d64b03e54ec5 timestamp=1s
INFO[0000] Deleting ES resource in source K8s cluster    name=elasticsearch-sample namespace=default timestamp=1s
INFO[0000] Force-deleting Pod in source K8s cluster      name=elasticsearch-sample-es-default-0 namespace=default timestamp=1s
INFO[0001] Force-deleting Pod in source K8s cluster      name=elasticsearch-sample-es-default-1 namespace=default timestamp=1s
INFO[0001] Force-deleting Pod in source K8s cluster      name=elasticsearch-sample-es-default-2 namespace=default timestamp=1s
INFO[0001] Creating PV in target cluster (same backing CSP volume)  name=pvc-fd2aa71e-a0b9-4b0c-8437-f4dc27ba65c9 timestamp=2s
INFO[0001] Creating PV in target cluster (same backing CSP volume)  name=pvc-9af782fd-27ed-42a3-b563-12dea569432f timestamp=2s
INFO[0001] Creating PV in target cluster (same backing CSP volume)  name=pvc-b38e0b86-d884-4503-bb7d-d64b03e54ec5 timestamp=2s
INFO[0001] Creating Elasticsearch in target cluster      name=elasticsearch-sample namespace=default timestamp=2s
INFO[0001] Waiting for all volumes to be bound and Pods to be running in target cluster  namespace=default timestamp=2s
INFO[0047] Waiting for Elasticsearch UUID to be reported  name=elasticsearch-sample namespace=default timestamp=47s
INFO[0062] Cluster UUID successfully preserved!          UUID=M3vlubgASvmpgCuxtmiSrQ name=elasticsearch-sample namespace=default timestamp=1m2s
INFO[0062] Deleting PV in source cluster                 name=pvc-fd2aa71e-a0b9-4b0c-8437-f4dc27ba65c9 timestamp=1m2s
INFO[0062] Deleting PV in source cluster                 name=pvc-9af782fd-27ed-42a3-b563-12dea569432f timestamp=1m2s
INFO[0062] Deleting PV in source cluster                 name=pvc-b38e0b86-d884-4503-bb7d-d64b03e54ec5 timestamp=1m2s
INFO[0062] Migration successful!                         timestamp=1m3s
```
