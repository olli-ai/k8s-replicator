package replicate

import (
	"encoding/base64"
	"encoding/json"
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
	// TODO: test delete option (impossible with the current implementation of fake client)
	_, err = secrets.Get("test-clear", metav1.GetOptions{})
	require.Error(t, err)
}

func decodeSecret(t *testing.T, secret *v1.Secret, name string) (map[string]string, map[string]bool) {
	data := map[string]string{}
	keys := map[string]bool{}
	for key, value := range secret.StringData {
		data[key] = value
		keys[key] = true
	}
	for key, value := range secret.Data {
		assert.NotContainsf(t, data, key, "already exists %s", name)
		buff := make([]byte, base64.StdEncoding.DecodedLen(len(value)))
		length, err := base64.StdEncoding.Decode(buff, value)
		assert.NoError(t, err, name)
		data[key] = string(buff[:length])
		keys[key] = true
	}
	return data, keys
}

func TestSecret_typesEmpty(t *testing.T) {
	example := []struct{
		name   string
		stype  v1.SecretType
		keys   map[string]bool
		values map[string]string
		check  func(t *testing.T, data map[string]string)
	}{{
		"opaque",
		v1.SecretTypeOpaque,
		map[string]bool{},
		nil,
		nil,
	}, {
		"unknown",
		v1.SecretType("unknown"),
		map[string]bool{},
		nil,
		nil,
	}, {
		".dockercfg",
		v1.SecretTypeDockercfg,
		map[string]bool{v1.DockerConfigKey: true},
		nil,
		func(t *testing.T, data map[string]string) {
			err := json.Unmarshal([]byte(data[v1.DockerConfigKey]), &map[string]interface{}{})
			assert.NoError(t, err, "json .dockercfg")
		},
	}, {
		".docker/config.json",
		v1.SecretTypeDockerConfigJson,
		map[string]bool{v1.DockerConfigJsonKey: true},
		nil,
		func(t *testing.T, data map[string]string) {
			err := json.Unmarshal([]byte(data[v1.DockerConfigJsonKey]), &map[string]interface{}{})
			assert.NoError(t, err, "json .docker/config.json")
		},
	}, {
		"basic-auth",
		v1.SecretTypeBasicAuth,
		map[string]bool{v1.BasicAuthUsernameKey: true, v1.BasicAuthPasswordKey: true},
		map[string]string{v1.BasicAuthUsernameKey: ""},
		func(t *testing.T, data map[string]string) {
			assert.GreaterOrEqual(t, len(data[v1.BasicAuthPasswordKey]), 100, "long password basic-auth")
		},
	}, {
		"ssh-auth",
		v1.SecretTypeSSHAuth,
		map[string]bool{v1.SSHAuthPrivateKey: true},
		nil,
		func(t *testing.T, data map[string]string) {
			assert.NotEqual(t, "", data[v1.SSHAuthPrivateKey], "not empty key ssh-auth")
			assert.LessOrEqual(t, len(data[v1.SSHAuthPrivateKey]), 10, "no key ssh-auth")
		},
	}, {
		"tls",
		v1.SecretTypeTLS,
		map[string]bool{v1.TLSCertKey: true, v1.TLSPrivateKeyKey: true},
		map[string]string{v1.TLSCertKey: "", v1.TLSPrivateKeyKey: ""},
		nil,
	}}
	for _, example := range example {
		replicator, watcher := createReplicator(_secretActions, "test-ns")
		require.Equalf(t, 0, len(watcher.Actions), "len(actions) %s", example.name)
		secrets := replicator.client.CoreV1().Secrets("test-ns")

		secret1, err := secrets.Create(&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-ns",
				Name: "secret1",
			},
			Type: example.stype,
			Data: MB{
				"test64": []byte("dGVzdDY0"),
			},
		})
		require.NoError(t, err, example.name)
		require.Equalf(t, 1, len(watcher.Actions), "len(actions) %s", example.name)

		update1, err := _secretActions.Clear(replicator.client, secret1, M{})
		require.NoError(t, err, example.name)
		require.Equalf(t, 2, len(watcher.Actions), "len(actions) %s", example.name)

		stored1, err := secrets.Get("secret1", metav1.GetOptions{})
		require.NoError(t, err, example.name)
		require.Equalf(t, 3, len(watcher.Actions), "len(actions) %s", example.name)
		assert.Equal(t, update1, stored1, example.name)

		data1, keys1 := decodeSecret(t, stored1, example.name)
		if example.keys != nil {
			assert.Equalf(t, example.keys, keys1, "keys %s", example.name)
		}
		for key, value := range example.values {
			if assert.Containsf(t, data1, key, "data %s", example.name) {
				assert.Equalf(t, value, data1[key], "data %s", example.name)
			}
		}
		if example.check != nil {
			example.check(t, data1)
		}

		secret2, err := secrets.Create(&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-ns",
				Name: "secret2",
			},
			Type: example.stype,
			Data: MB{
				"test64": []byte("dGVzdDY0"),
			},
		})
		require.NoError(t, err, example.name)
		require.Equalf(t, 4, len(watcher.Actions), "len(actions) %s", example.name)

		meta3 := &metav1.ObjectMeta{
			Namespace: "test-ns",
			Name: "secret3",
		}
		update3, err := _secretActions.Install(replicator.client, meta3, secret2, nil)
		require.NoError(t, err, example.name)
		require.Equalf(t, 5, len(watcher.Actions), "len(actions) %s", example.name)

		stored3, err := secrets.Get(meta3.Name, metav1.GetOptions{})
		require.NoError(t, err, example.name)
		require.Equalf(t, 6, len(watcher.Actions), "len(actions) %s", example.name)
		assert.Equal(t, update3, stored3)

		data3, keys3 := decodeSecret(t, stored3, example.name)
		if example.keys != nil {
			assert.Equalf(t, example.keys, keys3, "keys %s", example.name)
		}
		for key, value := range example.values {
			if assert.Containsf(t, data3, key, "data %s", example.name) {
				assert.Equalf(t, value, data3[key], "data %s", example.name)
			}
		}
		if example.check != nil {
			example.check(t, data3)
		}

		if example.stype == v1.SecretTypeBasicAuth {
			assert.NotEqual(t, data1[v1.BasicAuthPasswordKey], data3[v1.BasicAuthPasswordKey], "different password basic-auth")
		}
	}

}

func TestNewSecretReplicator(t *testing.T) {
	resyncPeriod := time.Hour
	sleep := 500 * time.Millisecond
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
	replicator := NewSecretReplicator(client, ReplicatorOptions{AllowAll: true}, resyncPeriod)
	replicator.Start()
	_, err := client.CoreV1().Secrets("from-ns").Create(&v1.Secret{
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
	time.Sleep(sleep)

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
	time.Sleep(sleep)
	secret, err = client.CoreV1().Secrets("target-1").Get("target", metav1.GetOptions{})
	assert.Error(t, err, "target-1/target")
	secret, err = client.CoreV1().Secrets("target-2").Get("target", metav1.GetOptions{})
	assert.Error(t, err, "target-2/target")
}
