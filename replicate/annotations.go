package replicate

import (
	"strings"
)

// Annotations that are used to specify this controller's behaviour
var (
	// ReplicateFromAnnotation tells to replicate from a source object to this object
	ReplicateFromAnnotation         = "replicate-from"
	// ReplicateToAnnotation tells to replicate this object to a target object(s)
	ReplicateToAnnotation           = "replicate-to"
	// ReplicateToNsAnnotation tells to replicate this object to a target namespace(s)
	ReplicateToNsAnnotation         = "replicate-to-namespaces"
	// ReplicateOnceAnnotation tells to replicate only once
	ReplicateOnceAnnotation         = "replicate-once"
	// ReplicateOnceVersionAnnotation tells to replicate once again when the annotation's value changes
	ReplicateOnceVersionAnnotation  = "replicate-once-version"
	// ReplicatedAtAnnotation stores when this object was replicated
	ReplicatedAtAnnotation          = "replicated-at"
	// ReplicatedByAnnotation stores which object created this replication
	ReplicatedByAnnotation          = "replicated-by"
	// ReplicatedFromVersionAnnotation stores the resource version of the source when replicated to this object
	ReplicatedFromVersionAnnotation = "replicated-from-version"
	// ReplicatedFromOriginAnnotation stores the object from which the data originates
	ReplicatedFromOriginAnnotation  = "replicated-from-origin"
	// ReplicationAllowedAnnotation explicitely allows replication
	ReplicationAllowedAnnotation    = "replication-allowed"
	// ReplicationAllowedNsAnnotation explicitely allows replication to the specified namespace(s)
	ReplicationAllowedNsAnnotation  = "replication-allowed-namespaces"
	// ReplicatedFromAllowedAnnotation stores the replication permissions of the source
	ReplicatedFromAllowedAnnotation  = "replicated-from-allowed"
)

var annotationsPrefix = ""

var annotationRefs = map[string]*string{
	ReplicateFromAnnotation:         &ReplicateFromAnnotation,
	ReplicateToAnnotation:           &ReplicateToAnnotation,
	ReplicateToNsAnnotation:         &ReplicateToNsAnnotation,
	ReplicateOnceAnnotation:         &ReplicateOnceAnnotation,
	ReplicateOnceVersionAnnotation:  &ReplicateOnceVersionAnnotation,
	ReplicatedAtAnnotation:          &ReplicatedAtAnnotation,
	ReplicatedByAnnotation:          &ReplicatedByAnnotation,
	ReplicatedFromVersionAnnotation: &ReplicatedFromVersionAnnotation,
	ReplicatedFromOriginAnnotation:  &ReplicatedFromOriginAnnotation,
	ReplicationAllowedAnnotation:    &ReplicationAllowedAnnotation,
	ReplicationAllowedNsAnnotation:  &ReplicationAllowedNsAnnotation,
	ReplicatedFromAllowedAnnotation: &ReplicatedFromAllowedAnnotation,
}

// PrefixAnnotations sets the prefix of all the annotations
func PrefixAnnotations(prefix string){
	if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}
	annotationsPrefix = prefix
	for suffix, annotation := range annotationRefs {
		*annotation = prefix + suffix
	}
}

// UnknownAnnotations returns the list of the unknown annotations with the same prefix
func UnknownAnnotations(annotations map[string]string) []string {
	var unknown []string = nil
	if annotationsPrefix != "" {
		for key := range annotations {
			if annotation := strings.TrimPrefix(key, annotationsPrefix); annotation == key {
			} else if _, ok := annotationRefs[annotation]; !ok {
				unknown = append(unknown, key)
			}
		}
	}
	return unknown
}
