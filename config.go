package main

import "time"

type flags struct {
	AnnotationsPrefix string
	KubeConfig        string
	ResyncPeriodS     string
	ResyncPeriod      time.Duration
	ReplicatorsS      string
	Replicators       []string
	LabelsS           string
	Labels            map[string]string
	StatusAddress     string
	AllowAll          bool
	IgnoreUnknown     bool
}
