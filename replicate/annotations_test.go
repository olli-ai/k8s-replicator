package replicate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixAnnotations(t *testing.T) {
	assert.Equal(t, ReplicateFromAnnotation, "replicate-from")
	PrefixAnnotations("test1")
	assert.Equal(t, ReplicateFromAnnotation, "test1/replicate-from")
	PrefixAnnotations("test2/")
	assert.Equal(t, ReplicateFromAnnotation, "test2/replicate-from")
	PrefixAnnotations("")
	assert.Equal(t, ReplicateFromAnnotation, "replicate-from")
}
