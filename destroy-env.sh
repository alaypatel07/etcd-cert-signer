kubectl delete secret etcd-ca etcd-1-peer etcd-1-server etcd-2-peer etcd-2-server etcd-3-peer etcd-3-server
kubectl delete secret etcd-ca
kubectl delete pod etcd-1 etcd-2 etcd-3 etcd-certs