package replicate

import (
	"regexp"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_targetPattern(t *testing.T) {
	examples := []struct{
		name      string
		pattern   string
		namespace string
		matches   bool
	}{{
		"number matches 0",
		"number-[0-9]+",
		"number-0",
		true,
	},{
		"number matches 123",
		"number-[0-9]+",
		"number-123",
		true,
	},{
		"number doesn't match abc",
		"number-[0-9]+",
		"number-abc",
		false,
	},{
		"number doesn't match suffix",
		"number-[0-9]+",
		"test-number-123",
		false,
	},{
		"number doesn't match prefix",
		"number-[0-9]+",
		"number-123-test",
		false,
	},{
		"option matches abc",
		"(abc|def)",
		"abc",
		true,
	},{
		"option matches def",
		"(abc|def)",
		"def",
		true,
	},{
		"option doesn't match prefix",
		"(abc|def)",
		"test-def",
		false,
	},{
		"option doesn't match suffix",
		"(abc|def)",
		"abc-test",
		false,
	}}
	namespaces := map[string][]string{}
	paths := map[string][]string{}
	for _, example := range examples {
		pattern, err := regexp.Compile(`^(?:`+example.pattern+`)$`)
		require.NoError(t, err, example.name)
		target := targetPattern{pattern, "my-name"}
		assert.Equal(t, example.matches, target.Match(&metav1.ObjectMeta{
			Name:      "my-name",
			Namespace: example.namespace,
		}), example.name)
		assert.False(t, target.Match(&metav1.ObjectMeta{
			Name:      "other-name",
			Namespace: example.namespace,
		}), example.name)
		assert.Equal(t, example.matches,
			target.MatchString(example.namespace+"/my-name"), example.name)
		assert.False(t,
			target.MatchString(example.namespace+"/other-name"), example.name)
		if example.matches {
			assert.Equal(t,example.namespace+"/my-name",
				target.MatchNamespace(example.namespace), example.name)
		} else {
			assert.Equal(t, "",
				target.MatchNamespace(example.namespace), example.name)
		}
		namespaces[example.pattern] = append(namespaces[example.pattern],
			example.namespace)
		if example.matches {
			paths[example.pattern] = append(paths[example.pattern],
				example.namespace+"/my-name")
		}
	}
	for p, ns := range namespaces {
		pattern, err := regexp.Compile(`^(?:`+p+`)$`)
		require.NoError(t, err, p)
		target := targetPattern{pattern, "my-name"}
		assert.Equal(t, paths[p], target.Targets(ns), p)
	}
}

func Test_isReplicationAllowed(t *testing.T) {
	type M = map[string]string
	examples := []struct{
		name        string
		allowAll    bool
		annotations map[string]string
		namespace   string
		allowed     bool
		disallowed  bool
	}{{
		"nothing",
		false,
		M{},
		"target-ns",
		false,
		true,
	}, {
		"allow all",
		true,
		M{},
		"target-ns",
		true,
		false,
	}, {
		"explicit disallow",
		true,
		M{ReplicationAllowedAnnotation: "false"},
		"target-ns",
		false,
		true,
	}, {
		"invalid allow annotation",
		true,
		M{ReplicationAllowedAnnotation: "other"},
		"target-ns",
		false,
		false,
	}, {
		"explicit disallow namespace",
		true,
		M{ReplicationAllowedNsAnnotation: "other-ns"},
		"target-ns",
		false,
		true,
	}, {
		"invalid namespace annotation",
		true,
		M{ReplicationAllowedNsAnnotation: "((("},
		"target-ns",
		false,
		false,
	}, {
		"explicit allow",
		false,
		M{ReplicationAllowedAnnotation: "true"},
		"target-ns",
		true,
		false,
	}, {
		"explicit allow namespace",
		false,
		M{ReplicationAllowedNsAnnotation: "target-ns"},
		"target-ns",
		true,
		false,
	}, {
		"explicit allow and allow namespace",
		false,
		M{
			ReplicationAllowedAnnotation: "true",
			ReplicationAllowedNsAnnotation: "target-ns",
		},
		"target-ns",
		true,
		false,
	}, {
		"allow but not namespace",
		false,
		M{
			ReplicationAllowedAnnotation: "true",
			ReplicationAllowedNsAnnotation: "other-ns",
		},
		"target-ns",
		false,
		true,
	}, {
		"allow namespace pattern",
		false,
		M{ReplicationAllowedNsAnnotation: "number-[0-9]+"},
		"number-123",
		true,
		false,
	}, {
		"disallow namespace pattern",
		false,
		M{ReplicationAllowedNsAnnotation: "number-[0-9]+"},
		"number-abc",
		false,
		true,
	}, {
		"allow namespace list",
		false,
		M{ReplicationAllowedNsAnnotation: "abc,def,ghi"},
		"def",
		true,
		false,
	}, {
		"disallow namespace list",
		false,
		M{ReplicationAllowedNsAnnotation: "abc,def,ghi"},
		"xyz",
		false,
		true,
	}, {
		"allow pattern list",
		false,
		M{ReplicationAllowedNsAnnotation: "abc-[0-9]+,def-[0-9]+,ghi-[0-9]+"},
		"def-123",
		true,
		false,
	}, {
		"disallow pattern list",
		false,
		M{ReplicationAllowedNsAnnotation: "abc-[0-9]+,def-[0-9]+,ghi-[0-9]+"},
		"def-abc",
		false,
		true,
	}, {
		"allow mix 1",
		false,
		M{ReplicationAllowedNsAnnotation: "number-other,number-[0-9]+"},
		"number-other",
		true,
		false,
	}, {
		"allow mix 2",
		false,
		M{ReplicationAllowedNsAnnotation: "number-other,number-[0-9]+"},
		"number-123",
		true,
		false,
	}}
	for _, example := range examples {
		props := &ReplicatorProps{
			Name: "test",
			ReplicatorOptions: ReplicatorOptions{
				AllowAll: example.allowAll,
			},
		}
		target := &metav1.ObjectMeta{
			Name:      "target",
			Namespace: example.namespace,
		}
		source := &metav1.ObjectMeta{
			Name:        "source",
			Namespace:   "source-ns",
			Annotations: example.annotations,
		}
		ok, nok, err := props.isReplicationAllowed(target, source)
		assert.Equal(t, example.allowed, ok, example.name)
		assert.Equal(t, example.disallowed, nok, example.name)
		if example.allowed {
			assert.NoError(t, err, example.name)
		} else {
			assert.Error(t, err, example.name)
		}
	}
}

func Test_needsDataUpdate(t *testing.T) {
	type M = map[string]string
	examples := []struct{
		name              string
		sourceAnnotations map[string]string
		targetAnnotations map[string]string
		update            bool
		once              bool
	}{{
		"never replicated",
		nil,
		nil,
		true,
		false,
	}, {
		"wrong source annotation",
		M{ReplicateOnceAnnotation: "other"},
		M{ReplicatedFromVersionAnnotation: "other"},
		false,
		false,
	}, {
		"wrong target annotation",
		nil,
		M{
			ReplicateOnceAnnotation: "other",
			ReplicatedFromVersionAnnotation: "other",
		},
		false,
		false,
	}, {
		"different resource version",
		nil,
		M{ReplicatedFromVersionAnnotation: "other"},
		true,
		false,
	}, {
		"same resource version",
		nil,
		M{ReplicatedFromVersionAnnotation: "test"},
		false,
		false,
	}, {
		"replicate once source",
		M{ReplicateOnceAnnotation: "true"},
		M{ReplicatedFromVersionAnnotation: "other"},
		false,
		true,
	}, {
		"replicate once target",
		nil,
		M{
			ReplicateOnceAnnotation: "true",
			ReplicatedFromVersionAnnotation: "other",
		},
		false,
		true,
	}, {
		"replicate once source never replicated",
		M{ReplicateOnceAnnotation: "true"},
		nil,
		true,
		false,
	}, {
		"replicate once target never replicated",
		nil,
		M{ReplicateOnceAnnotation: "true"},
		true,
		false,
	}, {
		"replicate once no version",
		M{ReplicateOnceVersionAnnotation: "new"},
		M{
			ReplicateOnceAnnotation: "true",
			ReplicatedFromVersionAnnotation: "other",
		},
		true,
		false,
	}, {
		"replicate once version",
		M{ReplicateOnceVersionAnnotation: "older"},
		M{
			ReplicateOnceAnnotation: "true",
			ReplicatedFromVersionAnnotation: "other",
			ReplicateOnceVersionAnnotation: "later",
		},
		true,
		false,
	}}
	props := &ReplicatorProps{
		Name: "test",
	}
	for _, example := range examples {
		target := &metav1.ObjectMeta{
			Name:        "target",
			Namespace:   "target-ns",
			Annotations: example.targetAnnotations,
		}
		source := &metav1.ObjectMeta{
			Name:            "source",
			Namespace:       "source-ns",
			Annotations:     example.sourceAnnotations,
			ResourceVersion: "test",
		}
		update, once, err := props.needsDataUpdate(target, source)
		assert.Equal(t, example.update, update, example.name)
		assert.Equal(t, example.once, once, example.name)
		if example.update {
			assert.NoError(t, err, example.name)
		} else {
			assert.Error(t, err, example.name)
		}
	}
}

func Test_needsFromAnnotationsUpdate(t *testing.T) {
	type M = map[string]string
	examples := []struct{
		name              string
		sourceAnnotations map[string]string
		targetAnnotations map[string]string
		update            bool
		error             bool
	}{{
		"no from annotation",
		nil,
		nil,
		false,
		true,
	}, {
		"invalid from annotation",
		M{ReplicateFromAnnotation: "!!!"},
		nil,
		false,
		true,
	}, {
		"missing from annotation",
		M{ReplicateFromAnnotation: "data-ns/data"},
		nil,
		true,
		false,
	}, {
		"missing from annotation name only",
		M{ReplicateFromAnnotation: "data"},
		nil,
		true,
		false,
	}, {
		"different from annotation",
		M{ReplicateFromAnnotation: "data-ns/data"},
		M{ReplicateFromAnnotation: "other-ns/other"},
		true,
		false,
	}, {
		"same from annotation",
		M{ReplicateFromAnnotation: "data-ns/data"},
		M{ReplicateFromAnnotation: "data-ns/data"},
		false,
		false,
	}, {
		"invalid once annotation",
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "other",
		},
		M{ReplicateFromAnnotation: "data-ns/data"},
		false,
		true,
	}, {
		"missing once annotation",
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "true",
		},
		M{ReplicateFromAnnotation: "data-ns/data"},
		true,
		false,
	}, {
		"present once annotation",
		M{ReplicateFromAnnotation: "data-ns/data"},
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "true",
		},
		true,
		false,
	}, {
		"different once annotation",
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "true",
		},
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "false",
		},
		true,
		false,
	}, {
		"same once annotation",
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "true",
		},
		M{
			ReplicateFromAnnotation: "data-ns/data",
			ReplicateOnceAnnotation: "true",
		},
		false,
		false,
	}}
	props := &ReplicatorProps{
		Name: "test",
	}
	for _, example := range examples {
		target := &metav1.ObjectMeta{
			Name:        "target",
			Namespace:   "target-ns",
			Annotations: example.targetAnnotations,
		}
		source := &metav1.ObjectMeta{
			Name:            "source",
			Namespace:       "source-ns",
			Annotations:     example.sourceAnnotations,
		}
		update, err := props.needsFromAnnotationsUpdate(target, source)
		assert.Equal(t, example.update, update, example.name)
		if example.error {
			assert.Error(t, err, example.name)
		} else {
			assert.NoError(t, err, example.name)
		}
	}
}

func Test_needsAllowedAnnotationsUpdate(t *testing.T) {
	type M = map[string]string
	examples := []struct{
		name              string
		sourceAnnotations map[string]string
		targetAnnotations map[string]string
		update            bool
		error             bool
	}{{
		"invalid allowed annotation",
		M{ReplicationAllowedAnnotation: "other"},
		nil,
		false,
		true,
	}, {
		"invalid allowed namespaces annotation",
		M{ReplicationAllowedNsAnnotation: "((("},
		nil,
		false,
		true,
	}, {
		"no annotation",
		nil,
		nil,
		false,
		false,
	}, {
		"missing allowed",
		M{ReplicationAllowedAnnotation: "true"},
		nil,
		true,
		false,
	}, {
		"present allowed",
		nil,
		M{ReplicationAllowedAnnotation: "true"},
		true,
		false,
	}, {
		"same allowed",
		M{ReplicationAllowedAnnotation: "true"},
		M{ReplicationAllowedAnnotation: "true"},
		false,
		false,
	}, {
		"different allowed",
		M{ReplicationAllowedAnnotation: "true"},
		M{ReplicationAllowedAnnotation: "false"},
		true,
		false,
	}, {
		"missing allowed namespace",
		M{ReplicationAllowedNsAnnotation: "other-ns"},
		nil,
		true,
		false,
	}, {
		"present allowed namespace",
		nil,
		M{ReplicationAllowedNsAnnotation: "other-ns"},
		true,
		false,
	}, {
		"same allowed namespace",
		M{ReplicationAllowedNsAnnotation: "other-ns"},
		M{ReplicationAllowedNsAnnotation: "other-ns"},
		false,
		false,
	}, {
		"different allowed namespace",
		M{ReplicationAllowedNsAnnotation: "other-ns"},
		M{ReplicationAllowedNsAnnotation: "different-ns"},
		true,
		false,
	}}
	props := &ReplicatorProps{
		Name: "test",
	}
	for _, example := range examples {
		target := &metav1.ObjectMeta{
			Name:        "target",
			Namespace:   "target-ns",
			Annotations: example.targetAnnotations,
		}
		source := &metav1.ObjectMeta{
			Name:            "source",
			Namespace:       "source-ns",
			Annotations:     example.sourceAnnotations,
		}
		update, err := props.needsAllowedAnnotationsUpdate(target, source)
		assert.Equal(t, example.update, update, example.name)
		if example.error {
			assert.Error(t, err, example.name)
		} else {
			assert.NoError(t, err, example.name)
		}
	}
}

func Test_isReplicatedBy(t *testing.T) {
	type M = map[string]string
	examples := []struct{
		name        string
		annotations map[string]string
		replicated  bool
	}{{
		"no annotation",
		nil,
		false,
	}, {
		"right annotation",
		M{ReplicatedByAnnotation: "source-ns/source"},
		true,
	}, {
		"wrong name",
		M{ReplicatedByAnnotation: "source-ns/other"},
		false,
	}, {
		"wrong namespace",
		M{ReplicatedByAnnotation: "other-ns/source"},
		false,
	}}
	props := &ReplicatorProps{
		Name: "test",
	}
	for _, example := range examples {
		target := &metav1.ObjectMeta{
			Name:        "target",
			Namespace:   "target-ns",
			Annotations: example.annotations,
		}
		source := &metav1.ObjectMeta{
			Name:      "source",
			Namespace: "source-ns",
		}
		replicated, err := props.isReplicatedBy(target, source)
		assert.Equal(t, example.replicated, replicated, example.name)
		if example.replicated {
			assert.NoError(t, err, example.name)
		} else {
			assert.Error(t, err, example.name)
		}
	}
}

func Test_isReplicatedTo(t *testing.T) {
	type M = map[string]string
	examples := []struct{
		name            string
		annotations     map[string]string
		targetNamespace string
		targetName      string
		replicated      bool
		error           bool
	}{{
		"no annotaion",
		nil,
		"target-ns",
		"target",
		false,
		false,
	}, {
		"invalid name",
		M{ReplicateToAnnotation: "!!!"},
		"target-ns",
		"target",
		false,
		true,
	}, {
		"invalid namespace",
		M{ReplicateToNsAnnotation: "((("},
		"target-ns",
		"target",
		false,
		true,
	}, {
		"same namespace",
		M{ReplicateToAnnotation: "target"},
		"source-ns",
		"target",
		true,
		false,
	}, {
		"same namespace wrong name",
		M{ReplicateToAnnotation: "target"},
		"source-ns",
		"other",
		false,
		false,
	}, {
		"same namespace wrong namespace",
		M{ReplicateToAnnotation: "target"},
		"other-ns",
		"target",
		false,
		false,
	}, {
		"name list",
		M{ReplicateToAnnotation: "abc,target,def"},
		"source-ns",
		"target",
		true,
		false,
	}, {
		"full path",
		M{ReplicateToAnnotation: "target-ns/target"},
		"target-ns",
		"target",
		true,
		false,
	}, {
		"full path wrong name",
		M{ReplicateToAnnotation: "target-ns/target"},
		"target-ns",
		"other",
		false,
		false,
	}, {
		"full path wrong namespace",
		M{ReplicateToAnnotation: "target-ns/target"},
		"other-ns",
		"target",
		false,
		false,
	}, {
		"same name",
		M{ReplicateToNsAnnotation: "target-ns"},
		"target-ns",
		"source",
		true,
		false,
	}, {
		"same name wrong name",
		M{ReplicateToNsAnnotation: "target-ns"},
		"target-ns",
		"other",
		false,
		false,
	}, {
		"same name wrong namespace",
		M{ReplicateToNsAnnotation: "target-ns"},
		"other-ns",
		"source",
		false,
		false,
	}, {
		"namespace list",
		M{ReplicateToNsAnnotation: "abc,target-ns,def"},
		"target-ns",
		"source",
		true,
		false,
	}, {
		"same name pattern namespace",
		M{ReplicateToNsAnnotation: "number-[0-9]+"},
		"number-123",
		"source",
		true,
		false,
	}, {
		"same name pattern namespace wrong name",
		M{ReplicateToNsAnnotation: "number-[0-9]+"},
		"number-123",
		"other",
		false,
		false,
	}, {
		"same name pattern namespace wrong namespace",
		M{ReplicateToNsAnnotation: "number-[0-9]+"},
		"number-abc",
		"source",
		false,
		false,
	}, {
		"pattern namespace list",
		M{ReplicateToNsAnnotation: "abc,number-[0-9]+,def"},
		"number-123",
		"source",
		true,
		false,
	}, {
		"full path pattern namespace",
		M{ReplicateToAnnotation: "number-[0-9]+/target"},
		"number-123",
		"target",
		true,
		false,
	}, {
		"full path pattern namespace wrong name",
		M{ReplicateToAnnotation: "number-[0-9]+/target"},
		"number-123",
		"other",
		false,
		false,
	}, {
		"full path pattern namespace wrong namespace",
		M{ReplicateToAnnotation: "number-[0-9]+/target"},
		"number-abc",
		"target",
		false,
		false,
	}, {
		"path pattern list",
		M{ReplicateToAnnotation: "abc/def,number-[0-9]+/target,ghi/jkl"},
		"number-123",
		"target",
		true,
		false,
	}}
	props := &ReplicatorProps{
		Name: "test",
	}
	for _, example := range examples {
		source := &metav1.ObjectMeta{
			Name:        "source",
			Namespace:   "source-ns",
			Annotations: example.annotations,
		}
		target := &metav1.ObjectMeta{
			Name:      example.targetName,
			Namespace: example.targetNamespace,
		}
		replicated, err := props.isReplicatedTo(source, target)
		assert.Equal(t, example.replicated, replicated, example.name)
		if example.error {
			assert.Error(t, err, example.name)
		} else {
			assert.NoError(t, err, example.name)
		}
	}
}

func Test_getReplicationTargets(t *testing.T) {
	type M = map[string]string
	type S = string
	type P struct{
		namespace string
		name      string
	}
	examples := []struct{
		name        string
		annotations map[string]string
		paths       []string
		patterns    []P
		error       bool
	}{{
		"name",
		M{ReplicateToAnnotation: "abc"},
		[]S{"source-ns/abc"},
		[]P{},
		false,
	}, {
		"name invalid name",
		M{ReplicateToAnnotation: "abc!"},
		nil,
		nil,
		true,
	}, {
		"names list",
		M{ReplicateToAnnotation: "abc,def,ghi"},
		[]S{"source-ns/abc", "source-ns/def", "source-ns/ghi"},
		[]P{},
		false,
	}, {
		"names list invalid name",
		M{ReplicateToAnnotation: "abc,def,[ghi]"},
		nil,
		nil,
		true,
	}, {
		"names list clean",
		M{ReplicateToAnnotation: "abc,source,def"},
		[]S{"source-ns/abc", "source-ns/def"},
		[]P{},
		false,
	}, {
		"path",
		M{ReplicateToAnnotation: "abc/def"},
		[]S{"abc/def"},
		[]P{},
		false,
	}, {
		"path invalid name",
		M{ReplicateToAnnotation: "abc/[def]"},
		nil,
		nil,
		true,
	}, {
		"path too long",
		M{ReplicateToAnnotation: "abc/def/ghi"},
		nil,
		nil,
		true,
	}, {
		"paths list",
		M{ReplicateToAnnotation: "abc/def,ghi/jkl,mno/pqr"},
		[]S{"abc/def", "ghi/jkl", "mno/pqr"},
		[]P{},
		false,
	}, {
		"paths list invalid name",
		M{ReplicateToAnnotation: "abc/def!,ghi/jkl,mno/pqr"},
		nil,
		nil,
		true,
	}, {
		"paths list too long",
		M{ReplicateToAnnotation: "abc/def,ghi/jkl,mno/pqr/stu"},
		nil,
		nil,
		true,
	}, {
		"paths list clean",
		M{ReplicateToAnnotation: "abc/def,source-ns/source,ghi/jkl"},
		[]S{"abc/def", "ghi/jkl"},
		[]P{},
		false,
	}, {
		"path pattern",
		M{ReplicateToAnnotation: "[abc]/def"},
		[]S{},
		[]P{{"[abc]", "def"}},
		false,
	}, {
		"path pattern invalid pattern",
		M{ReplicateToAnnotation: "[abc/def"},
		nil,
		nil,
		true,
	}, {
		"path pattern invalid name",
		M{ReplicateToAnnotation: "[abc]/def!"},
		nil,
		nil,
		true,
	}, {
		"path pattern invalid",
		M{ReplicateToAnnotation: "[abc/def]"},
		nil,
		nil,
		true,
	}, {
		"path pattern too long",
		M{ReplicateToAnnotation: "[abc]/def/ghi"},
		nil,
		nil,
		true,
	}, {
		"path patterns list",
		M{ReplicateToAnnotation: "[abc]/def,[ghi]/jkl,[mno]/pqr"},
		[]S{},
		[]P{{"[abc]", "def"}, {"[ghi]", "jkl"}, {"[mno]", "pqr"}},
		false,
	}, {
		"path patterns list invalid pattern",
		M{ReplicateToAnnotation: "[abc]/def,[ghi/jkl,[mno]/pqr"},
		nil,
		nil,
		true,
	}, {
		"path patterns list invalid name",
		M{ReplicateToAnnotation: "[abc]/def,[ghi]/jkl,[mno]/pqr!"},
		nil,
		nil,
		true,
	}, {
		"path patterns list too long",
		M{ReplicateToAnnotation: "[abc]/def/stu,[ghi]/jkl,[mno]/pqr"},
		nil,
		nil,
		true,
	}, {
		"names mix",
		M{ReplicateToAnnotation: "[abc]/def,ghi,jkl/mno,pqr"},
		[]S{"source-ns/ghi", "jkl/mno", "source-ns/pqr"},
		[]P{{"[abc]", "def"}},
		false,
	}, {
		"namespace",
		M{ReplicateToNsAnnotation: "abc"},
		[]S{"abc/source"},
		[]P{},
		false,
	}, {
		"namespaces list",
		M{ReplicateToNsAnnotation: "abc,def,ghi"},
		[]S{"abc/source", "def/source", "ghi/source"},
		[]P{},
		false,
	}, {
		"namespaces list clean",
		M{ReplicateToNsAnnotation: "abc,source-ns,def"},
		[]S{"abc/source", "def/source"},
		[]P{},
		false,
	}, {
		"pattern",
		M{ReplicateToNsAnnotation: "[abc]"},
		[]S{},
		[]P{{"[abc]", "source"}},
		false,
	}, {
		"pattern invalid pattern",
		M{ReplicateToNsAnnotation: "[abc"},
		nil,
		nil,
		true,
	}, {
		"patterns list",
		M{ReplicateToNsAnnotation: "[abc],[def],[ghi]"},
		[]S{},
		[]P{{"[abc]", "source"}, {"[def]", "source"}, {"[ghi]", "source"}},
		false,
	}, {
		"patterns list invalid pattern",
		M{ReplicateToNsAnnotation: "[abc],[def],[ghi"},
		nil,
		nil,
		true,
	}, {
		"patterns list invalid",
		M{ReplicateToNsAnnotation: "[abc,ghi]"},
		nil,
		nil,
		true,
	}, {
		"namespaces mix",
		M{ReplicateToNsAnnotation: "[abc],def,[ghi],jkl"},
		[]S{"def/source", "jkl/source"},
		[]P{{"[abc]", "source"}, {"[ghi]", "source"}},
		false,
	}, {
		"all mix",
		M{
			ReplicateToAnnotation: "abc,def/ghi,jkl,[mno]/pqr",
			ReplicateToNsAnnotation: "cba,[fed],igh,[lkj]",
		},
		[]S{"cba/abc", "igh/abc", "def/ghi", "cba/jkl", "igh/jkl"},
		[]P{{"[fed]", "abc"}, {"[lkj]", "abc"}, {"[fed]", "jkl"}, {"[lkj]", "jkl"}, {"[mno]", "pqr"}},
		false,
	}, {
		"unique",
		M{
			ReplicateToAnnotation: "abc,def/abc,[ghi]/abc,source,jkl/mno",
			ReplicateToNsAnnotation: "[abc],def,source-ns,[ghi],jkl",
		},
		[]S{"def/abc", "jkl/abc", "source-ns/abc", "def/source", "jkl/source", "jkl/mno"},
		[]P{{"[abc]", "abc"}, {"[ghi]", "abc"}, {"[abc]", "source"}, {"[ghi]", "source"}},
		false,
	}}
	props := &ReplicatorProps{
		Name: "test",
	}
	for _, example := range examples {
		source := &metav1.ObjectMeta{
			Name:        "source",
			Namespace:   "source-ns",
			Annotations: example.annotations,
		}
		paths, patterns, err := props.getReplicationTargets(source)
		if example.error {
			assert.Nil(t, paths, example.name)
			assert.Nil(t, patterns, example.name)
			assert.Error(t, err, example.name)
		} else {
			assert.ElementsMatch(t, example.paths, paths, example.name)
			patternsCopy := make([]P, len(patterns))
			for i, p := range patterns {
				patternsCopy[i] = P{
					p.namespace.String(),
					p.name,
				}
			}
			for i, p := range example.patterns {
				example.patterns[i].namespace = `^(?:`+p.namespace+`)$`
			}
			assert.ElementsMatch(t, example.patterns, patternsCopy, example.name)
		}
	}
}

func Test_resolveAnnotation(t *testing.T) {
	examples := []struct{
		name       string
		annotation string
		expected   string
	}{{
		"absent",
		"",
		"",
	}, {
		"name",
		"my-name",
		"object-ns/my-name",
	}, {
		"path",
		"my-ns/my-name",
		"my-ns/my-name",
	}}
	for _, example := range examples {
		object := &metav1.ObjectMeta{
			Name:        "object",
			Namespace:   "object-ns",
			Annotations: map[string]string{},
		}
		if example.annotation != "" {
			object.Annotations["test-annotation"] = example.annotation
		}
		out, ok := resolveAnnotation(object, "test-annotation")
		assert.Equal(t, example.expected, out, example.name)
		assert.Equal(t, example.expected != "", ok, example.name)
	}
}

func Test_annotationRefersTo(t *testing.T) {
	examples := []struct{
		name            string
		annotation      string
		targetName      string
		targetNamespace string
		refers          bool
	}{{
		"no annotation",
		"",
		"target",
		"source-ns",
		false,
	}, {
		"name",
		"target",
		"target",
		"source-ns",
		true,
	}, {
		"wrong name",
		"other",
		"target",
		"source-ns",
		false,
	}, {
		"path",
		"target-ns/target",
		"target",
		"target-ns",
		true,
	}, {
		"wrong namespace",
		"other-ns/target",
		"target",
		"target-ns",
		false,
	}}
	for _, example := range examples {
		source := &metav1.ObjectMeta{
			Name:        "source",
			Namespace:   "source-ns",
			Annotations: map[string]string{},
		}
		if example.annotation != "" {
			source.Annotations["test-annotation"] = example.annotation
		}
		target := &metav1.ObjectMeta{
			Name:      example.targetName,
			Namespace: example.targetNamespace,
		}
		refers := annotationRefersTo(source, "test-annotation", target)
		assert.Equal(t, example.refers, refers, example.name)
	}
}
