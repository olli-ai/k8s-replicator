package replicate

// Annotations that are used to control this controller's behaviour
var (
	ReplicateFromAnnotation         = "replicate-from"
	ReplicateToAnnotation           = "replicate-to"
	ReplicateToNsAnnotation         = "replicate-to-namespaces"
	ReplicateOnceAnnotation         = "replicate-once"
	ReplicateOnceVersionAnnotation  = "replicate-once-version"
	ReplicatedAtAnnotation          = "replicated-at"
	ReplicatedByAnnotation          = "replicated-by"
	ReplicatedFromVersionAnnotation = "replicated-from-version"
	ReplicationAllowedAnnotation    = "replication-allowed"
	ReplicationAllowedNsAnnotation  = "replication-allowed-namespaces"
)

var annotaions = map[string]*string{
	ReplicateFromAnnotation:         &ReplicateFromAnnotation,
	ReplicateToAnnotation:           &ReplicateToAnnotation,
	ReplicateToNsAnnotation:         &ReplicateToNsAnnotation,
	ReplicateOnceAnnotation:         &ReplicateOnceAnnotation,
	ReplicateOnceVersionAnnotation:  &ReplicateOnceVersionAnnotation,
	ReplicatedAtAnnotation:          &ReplicatedAtAnnotation,
	ReplicatedByAnnotation:          &ReplicatedByAnnotation,
	ReplicatedFromVersionAnnotation: &ReplicatedFromVersionAnnotation,
	ReplicationAllowedAnnotation:    &ReplicationAllowedAnnotation,
	ReplicationAllowedNsAnnotation:  &ReplicationAllowedNsAnnotation,
}

// PrefixAnnotations sets the prefix of all the annotations
func PrefixAnnotations(prefix string){
	if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}
	for suffix, annotation := range annotaions {
		*annotation = prefix + suffix
	}
}
