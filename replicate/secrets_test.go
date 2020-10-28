package replicate

import (
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecret_GetMeta(t *testing.T) {
	object := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test",
			Annotations: M{
				"test-annotation": "test",
			},
		},
		Type: "test-type",
		Data: MB{
			"test": []byte("test-data"),
		},
	}
	copy := object.ObjectMeta.DeepCopy()
	assert.Equal(t, copy, _secretActions.GetMeta(object))
}

func TestSecret_Update(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	old, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-update",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("old"),
			"test-old": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "data",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	annotations := M{
		"test-annotation": "new",
		"test-new": "annotation",
	}

	old2 := old.DeepCopy()
	source2 := source.DeepCopy()
	store, err := _secretActions.Update(replicator.client, old2, source2, annotations)
	require.NoError(t, err)
	assert.Equal(t, old, old2, "old changed")
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-update", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-update",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.Secret)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestSecret_Clear(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	todo, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-clear",
			Annotations: M{
				"test-annotation": "todo",
				"test-todo": "annotation",
			},
			ResourceVersion: "todo",
		},
		Type: "todo",
		Data: MB{
			"test-data": []byte("todo"),
			"test-todo": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	annotations := M{
		"test-annotation": "done",
		"test-done": "annotation",
	}

	todo2 := todo.DeepCopy()
	store, err := _secretActions.Clear(replicator.client, todo2, annotations)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-clear", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-clear",
			Annotations: M{
				"test-annotation": "done",
				"test-done": "annotation",
			},
			ResourceVersion: "todo",
		},
		Type: "todo",
		Data: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.Secret)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestSecret_install_create_empty(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "",
	}

	source2 := source.DeepCopy()
	store, err := _secretActions.Install(replicator.client, meta, source2, nil)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")
	require.Equal(t, "create", watcher.Actions[0].GetVerb())
	sent, ok := watcher.Actions[0].(CreateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "",
		},
		Type: "source",
		Data: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.Secret)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestSecret_install_update_empty(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	_, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("old"),
			"test-old": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "old",
	}

	source2 := source.DeepCopy()
	store, err := _secretActions.Install(replicator.client, meta, source2, nil)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "source",
		Data: nil,
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.Secret)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}

func TestSecret_install_create_data(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	copy := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-copy",
			Annotations: M{
				"test-annotation": "copy",
				"test-copy": "annotation",
			},
			ResourceVersion: "copy",
		},
		Type: "copy",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "",
	}

	source2 := source.DeepCopy()
	copy2 := copy.DeepCopy()
	store, err := _secretActions.Install(replicator.client, meta, source2, copy2)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	assert.Equal(t, copy, copy2, "copy changed")
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")
	require.Equal(t, "create", watcher.Actions[0].GetVerb())
	sent, ok := watcher.Actions[0].(CreateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.Secret)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestSecret_install_update_data(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	_, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "old",
				"test-old": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "old",
		Data: MB{
			"test-data": []byte("old"),
			"test-old": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-source",
			Annotations: M{
				"test-annotation": "source",
				"test-source": "annotation",
			},
			ResourceVersion: "source",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("source"),
			"test-source": []byte("data"),
		},
	}
	copy := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-copy",
			Annotations: M{
				"test-annotation": "copy",
				"test-copy": "annotation",
			},
			ResourceVersion: "copy",
		},
		Type: "copy",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
		},
	}
	meta := &metav1.ObjectMeta{
		Namespace: "test-ns",
		Name: "test-install",
		Annotations: M{
			"test-annotation": "new",
			"test-new": "annotation",
		},
		ResourceVersion: "old",
	}

	source2 := source.DeepCopy()
	copy2 := copy.DeepCopy()
	store, err := _secretActions.Install(replicator.client, meta, source2, copy2)
	require.NoError(t, err)
	assert.Equal(t, source, source2, "source changed")
	assert.Equal(t, copy, copy2, "copy changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "update", watcher.Actions[1].GetVerb())
	sent, ok := watcher.Actions[1].(UpdateAction).GetObject().(*v1.Secret)
	require.True(t, ok, "secret")
	new, err := secrets.Get("test-install", metav1.GetOptions{})
	require.NoError(t, err)

	expected := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-install",
			Annotations: M{
				"test-annotation": "new",
				"test-new": "annotation",
			},
			ResourceVersion: "old",
		},
		Type: "source",
		Data: MB{
			"test-data": []byte("copy"),
			"test-copy": []byte("data"),
		},
	}
	assert.Equal(t, expected, sent, "sent")
	expected.ObjectMeta.ResourceVersion = new.ObjectMeta.ResourceVersion
	assert.Equal(t, expected, new, "new")
	new, ok = store.(*v1.Secret)
	if assert.True(t, ok, "store") {
		assert.Equal(t, expected, new, "store")
	}
}

func TestSecret_Delete(t *testing.T) {
	replicator, watcher := createReplicator(_secretActions, "test-ns")
	require.Equal(t, 0, len(watcher.Actions), "len(actions)")
	secrets := replicator.client.CoreV1().Secrets("test-ns")

	todo, err := secrets.Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "test-delete",
			Annotations: M{
				"test-annotation": "todo",
				"test-todo": "annotation",
			},
			ResourceVersion: "todo",
		},
		Type: "todo",
		Data: MB{
			"test-data": []byte("todo"),
			"test-todo": []byte("data"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(watcher.Actions), "len(actions)")

	todo2 := todo.DeepCopy()
	err = _secretActions.Delete(replicator.client, todo2)
	require.NoError(t, err)
	assert.Equal(t, todo, todo2, "todo changed")
	require.Equal(t, 2, len(watcher.Actions), "len(actions)")
	require.Equal(t, "delete", watcher.Actions[1].GetVerb())
	require.Equal(t, "test-delete", watcher.Actions[1].(DeleteAction).GetName())
	// TODO: test delete option (impossible with the current implementation)
	_, err = secrets.Get("test-clear", metav1.GetOptions{})
	require.Error(t, err)
}

func TestNewSecretReplicator(t *testing.T) {
	hour, err := time.ParseDuration("1h")
	require.NoError(t, err)
	second, err := time.ParseDuration("0.5s")
	require.NoError(t, err)
	client := fake.NewSimpleClientset(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "source-ns",
			Name: "source",
		},
		Data: MB{
			"data": []byte("source"),
		},
	}, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target-1",
		},
	})
	replicator := NewSecretReplicator(client, ReplicatorOptions{AllowAll: true}, hour)
	replicator.Start()
	_, err = client.CoreV1().Secrets("from-ns").Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "from-ns",
			Name: "from",
			Annotations: M{
				ReplicateFromAnnotation: "source-ns/source",
			},
		},
	})
	require.NoError(t, err, "from-ns/from")
	_, err = client.CoreV1().Secrets("to-ns").Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "to-ns",
			Name: "to",
			Annotations: M{
				ReplicateToAnnotation: "target-[0-9]+/target",
				ReplicateFromAnnotation: "source-ns/source",
			},
		},
	})
	require.NoError(t, err, "to-ns/to")
	_, err = client.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target-2",
		},
	})
	require.NoError(t, err, "target-2")
	time.Sleep(second)

	secret, err := client.CoreV1().Secrets("from-ns").Get("from", metav1.GetOptions{})
	if assert.NoError(t, err, "from-ns/from") {
		assert.Equal(t, []byte("source"), secret.Data["data"], "from-ns/from")
	}
	secret, err = client.CoreV1().Secrets("target-1").Get("target", metav1.GetOptions{})
	if assert.NoError(t, err, "target-1/target") {
		assert.Equal(t, []byte("source"), secret.Data["data"], "target-1/target")
	}
	secret, err = client.CoreV1().Secrets("target-2").Get("target", metav1.GetOptions{})
	if assert.NoError(t, err, "target-2/target") {
		assert.Equal(t, []byte("source"), secret.Data["data"], "target-2/target")
	}

	err = client.CoreV1().Secrets("to-ns").Delete("to", &metav1.DeleteOptions{})
	require.NoError(t, err, "to-ns/to")
	time.Sleep(second)
	secret, err = client.CoreV1().Secrets("target-1").Get("target", metav1.GetOptions{})
	assert.Error(t, err, "target-1/target")
	secret, err = client.CoreV1().Secrets("target-2").Get("target", metav1.GetOptions{})
	assert.Error(t, err, "target-2/target")
}
