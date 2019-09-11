package controller

import (
	"github.com/alaypatel07/etcd-cert-signer/pkg/controller/etcdcertsigner"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, etcdcertsigner.Add)
}
