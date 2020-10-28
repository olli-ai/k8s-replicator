package replicate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixAnnotations(t *testing.T) {
	original := annotationsPrefix
	defer PrefixAnnotations(original)

	PrefixAnnotations("test1")
	assert.Equal(t, ReplicateFromAnnotation, "test1/replicate-from")
	PrefixAnnotations("test2/")
	assert.Equal(t, ReplicateFromAnnotation, "test2/replicate-from")
	PrefixAnnotations("")
	assert.Equal(t, ReplicateFromAnnotation, "replicate-from")
}

func TestUnknownAnnotations(t *testing.T) {
	original := annotationsPrefix
	defer PrefixAnnotations(original)

	PrefixAnnotations("test")
	unkown := UnknownAnnotations(M{
		"test/replicate-from": "any",
		"test/replicate-to": "any",
	})
	assert.Nil(t, unkown, "no unknown")

	PrefixAnnotations("test")
	unkown = UnknownAnnotations(M{
		"test/replicate-invalid": "any",
		"test/replicate-from": "any",
		"test/replicate-to": "any",
		"test/replicate-not-exists": "any",
	})
	assert.ElementsMatch(t, []string{"test/replicate-invalid", "test/replicate-not-exists"}, unkown, "2 unknown")

	PrefixAnnotations("")
	unkown = UnknownAnnotations(M{
		"test/replicate-invalid": "any",
		"test/replicate-from": "any",
		"test/replicate-to": "any",
		"test/replicate-not-exists": "any",
	})
	assert.Nil(t, unkown, "no prefix")
}
