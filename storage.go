package certmagickubestorage

import (
	"context"
	"os"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
	"github.com/jrhouston/k8slock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	namespace string = os.Getenv("POD_NAMESPACE")
	podname   string = os.Getenv("POD_NAME")
)

func init() {
	caddy.RegisterModule(storage{})
}

type storage struct {
	k8sClient *kubernetes.Clientset
	lockerMap map[string]*k8slock.Locker
}

func (storage) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "caddy.storage.kubernetes",
		New: func() caddy.Module {
			s := &storage{
				lockerMap: map[string]*k8slock.Locker{},
			}
			// in-cluster kubeconfig
			config, err := rest.InClusterConfig()
			if err != nil {
				panic(err)
			}
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				panic(err)
			}
			s.k8sClient = clientset

			return s
		},
	}
}

func (s *storage) Lock(ctx context.Context, name string) error {
	locker, err := k8slock.NewLocker(
		name,
		k8slock.Clientset(s.k8sClient),
		k8slock.TTL(time.Second*60),
		k8slock.RetryWaitDuration(time.Second*10),
		k8slock.ClientID(podname),
		k8slock.Namespace(namespace),
	)
	if err != nil {
		return err
	}

	s.lockerMap[name] = locker

	locker.Lock()

	return nil
}

func (s *storage) Unlock(ctx context.Context, name string) error {
	s.lockerMap[name].Unlock()
	return nil
}

func (s *storage) Store(ctx context.Context, key string, value []byte) error {
	secret := corev1.Secret(key, namespace)
	secret.Data = map[string][]byte{"data": value}
	secret.Labels = map[string]string{"updated_at": time.Now().String()}

	_, err := s.k8sClient.CoreV1().Secrets(namespace).Apply(ctx, secret, metav1.ApplyOptions{})
	return err
}

func (s *storage) Load(ctx context.Context, key string) ([]byte, error) {
	secret, err := s.k8sClient.CoreV1().Secrets(namespace).Get(ctx, key, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return secret.Data["data"], nil
}

func (s *storage) Delete(ctx context.Context, key string) error {
	return s.k8sClient.CoreV1().Secrets(namespace).Delete(ctx, key, metav1.DeleteOptions{})
}

func (s *storage) Exists(ctx context.Context, key string) bool {
	_, err := s.k8sClient.CoreV1().Secrets(namespace).Get(ctx, key, metav1.GetOptions{})
	return err == nil
}

func (s *storage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	secrets, err := s.k8sClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: map[string]string{}}),
	})
	if err != nil {
		return nil, err
	}

	secretsList := []string{}
	for _, v := range secrets.Items {
		secretsList = append(secretsList, v.Name)
	}

	return secretsList, nil
}

func (s *storage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {

	keyInfo := certmagic.KeyInfo{
		Key: key,
	}

	secret, err := s.k8sClient.CoreV1().Secrets(namespace).Get(ctx, key, metav1.GetOptions{})
	if err != nil {
		return keyInfo, err
	}

	keyInfo.Size = int64(len(secret.Data["data"]))

	return keyInfo, nil
}
