package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/olli-ai/kubernetes-replicator/liveness"
	"github.com/olli-ai/kubernetes-replicator/replicate"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var f flags

func init() {
	var err error
	flag.StringVar(&f.AnnotationsPrefix, "prefix", "v1.kubernetes-replicator.olli.com/", "prefix for all annotations")
	flag.StringVar(&f.Kubeconfig, "kubeconfig", "", "path to Kubernetes config file")
	flag.StringVar(&f.ResyncPeriodS, "resync-period", "30m", "resynchronization period")
	flag.StringVar(&f.StatusAddr, "status-addr", ":9102", "listen address for status and monitoring server")
	flag.BoolVar(&f.AllowAll, "allow-all", false, "allow replication of all secrets by default (CAUTION: only use when you know what you're doing)")
	flag.BoolVar(&f.IgnoreUnknown, "ignore-unknown", true, "unkown annotations with the same prefix do not raise an error")
	flag.Parse()

	replicate.PrefixAnnotations(f.AnnotationsPrefix)

	f.ResyncPeriod, err = time.ParseDuration(f.ResyncPeriodS)
	if err != nil {
		panic(err)
	}
}

type newReplicatorFunc func(kubernetes.Interface, replicate.ReplicatorOptions, time.Duration) replicate.Replicator

var newReplicatorFuncs []newReplicatorFunc = []newReplicatorFunc{
	replicate.NewConfigMapReplicator,
	replicate.NewSecretReplicator,
}

func main() {
	var config *rest.Config
	var err error
	var client kubernetes.Interface

	if f.Kubeconfig == "" {
		log.Printf("using in-cluster configuration")
		config, err = rest.InClusterConfig()
	} else {
		log.Printf("using configuration from '%s'", f.Kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", f.Kubeconfig)
	}

	if err != nil {
		panic(err)
	}

	client = kubernetes.NewForConfigOrDie(config)
	options := replicate.ReplicatorOptions{
		AllowAll:      f.AllowAll,
		IgnoreUnknown: f.IgnoreUnknown,
	}

	var replicators []replicate.Replicator
	for _, newReplicator := range(newReplicatorFuncs) {
		replicators = append(replicators, newReplicator(client, options, f.ResyncPeriod))
	}

	log.Printf("Starting replicators with prefix \"%s\"", f.AnnotationsPrefix)
	for _, replicator := range(replicators) {
		replicator.Start()
	}

	h := liveness.Handler{
		Replicators: replicators,
	}

	log.Printf("starting liveness monitor at %s", f.StatusAddr)

	http.Handle("/healthz", &h)
	http.ListenAndServe(f.StatusAddr, nil)
}
