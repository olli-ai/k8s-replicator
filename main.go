package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	flag.StringVar(&f.AnnotationsPrefix, "annotations-prefix", "v1.kubernetes-replicator.olli.com/", "prefix for all annotations")
	flag.StringVar(&f.KubeConfig, "kube-config", "", "path to Kubernetes config file")
	flag.StringVar(&f.ResyncPeriodS, "resync-period", "30m", "resynchronization period")
	flag.StringVar(&f.ReplicatorsS, "run-replicators", "all", "replicators to run")
	flag.StringVar(&f.LabelsS, "create-with-labels", "", "labels to add to created resources")
	flag.StringVar(&f.StatusAddress, "status-address", ":9102", "listen address for status and monitoring server")
	flag.BoolVar(&f.AllowAll, "allow-all", false, "allow replication of all secrets by default (CAUTION: only use when you know what you're doing)")
	flag.BoolVar(&f.IgnoreUnknown, "ignore-unknown", true, "unkown annotations with the same prefix do not raise an error")
	flag.Parse()

	replicate.PrefixAnnotations(f.AnnotationsPrefix)

	if f.ResyncPeriod, err = time.ParseDuration(f.ResyncPeriodS); err != nil {
		panic(fmt.Errorf("invalid --resync-period \"%s\": %s", f.ResyncPeriodS, err))
	}

	for _, replicator := range strings.Split(f.ReplicatorsS, ",") {
		if replicator = strings.Trim(replicator, " "); replicator != "" {
			f.Replicators = append(f.Replicators, strings.ToLower(replicator))
		}
	}

	f.Labels = map[string]string{}
	for _, labelValue := range strings.Split(f.LabelsS, ",") {
		labelValue = strings.Trim(labelValue, " ")
		if labelValue == "" {
			continue
		} else if parts := strings.Split(labelValue, "="); len(parts) != 2 {
		} else if label := strings.Trim(parts[0], " "); label == "" {
		} else if value := strings.Trim(parts[1], " "); value == "" {
		} else {
			f.Labels[label] = value
			continue
		}
		panic(fmt.Errorf("invalid --labels \"%s\": format label=value expected", labelValue))
	}
}

type newReplicatorFunc func(kubernetes.Interface, replicate.ReplicatorOptions, time.Duration) replicate.Replicator

// All the new replicator function, key must be lower case
var newReplicatorFuncs map[string]newReplicatorFunc = map[string]newReplicatorFunc{
	"configmap": replicate.NewConfigMapReplicator,
	"secret": replicate.NewSecretReplicator,
}

func main() {
	var config *rest.Config
	var err error
	var client kubernetes.Interface

	if f.KubeConfig == "" {
		log.Printf("using in-cluster configuration")
		config, err = rest.InClusterConfig()
	} else {
		log.Printf("using configuration from '%s'", f.KubeConfig)
		config, err = clientcmd.BuildConfigFromFlags("", f.KubeConfig)
	}
	if err != nil {
		panic(err)
	}

	client = kubernetes.NewForConfigOrDie(config)
	options := replicate.ReplicatorOptions{
		AllowAll:      f.AllowAll,
		IgnoreUnknown: f.IgnoreUnknown,
		Labels:        f.Labels,
	}

	selectedReplicatorFuncs := map[string]newReplicatorFunc{}
	for _, replicator := range(f.Replicators) {
		if replicator == "all" {
			for key, value := range newReplicatorFuncs {
				selectedReplicatorFuncs[key] = value
			}
		} else if value, ok := newReplicatorFuncs[replicator]; ok {
			selectedReplicatorFuncs[replicator] = value
		} else {
			panic(fmt.Errorf("no replicator %s", replicator))
		}
	}

	replicators := []replicate.Replicator{}
	for _, newReplicator := range(selectedReplicatorFuncs) {
		replicators = append(replicators, newReplicator(client, options, f.ResyncPeriod))
	}

	log.Printf("Starting replicators with prefix \"%s\"", f.AnnotationsPrefix)
	for _, replicator := range(replicators) {
		replicator.Start()
	}

	h := liveness.Handler{
		Replicators: replicators,
	}

	log.Printf("starting liveness monitor at %s", f.StatusAddress)

	http.Handle("/healthz", &h)
	http.ListenAndServe(f.StatusAddress, nil)
}
