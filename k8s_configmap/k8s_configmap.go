// coordx/k8s_configmap: K8s ConfigMap 服务协调驱动
package k8sconfigmap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"go.opentelemetry.io/otel/attribute"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type registerImpl struct {
	client *kubernetes.Clientset
	ns     string
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewRegister 创建 K8s ConfigMap 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
	var err error
	kubeconfig := cfg.Addr
	if kubeconfig == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	kubeCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("coordx/k8s: load kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		return nil, fmt.Errorf("coordx/k8s: create client: %w", err)
	}
	ns := cfg.Cluster
	if ns == "" {
		ns = "default"
	}
	r := &registerImpl{
		client: client,
		ns:     ns,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/k8s_configmap", "coordx"),
	}
	r.logger.InfoContext(context.Background(), "coordx/k8s_configmap initialized", logx.String("ns", ns))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.register")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "k8s_configmap"),
		attribute.String("coord.service", service),
		attribute.String("coord.namespace", r.ns),
		attribute.String("coord.instance_id", inst.ID),
	)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ns,
			Name:      service,
			Labels:    map[string]string{"coordx.io/service": service},
		},
		Data: map[string]string{
			"instance-id": inst.ID,
			"address":     inst.Addr,
			"port":        fmt.Sprintf("%d", inst.Port),
			"healthy":     fmt.Sprintf("%t", inst.Healthy),
			"created-at":  fmt.Sprintf("%d", time.Now().Unix()),
		},
	}
	if _, err := r.client.CoreV1().ConfigMaps(r.ns).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/k8s_configmap register failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/k8s_configmap register done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.deregister")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "k8s_configmap"),
		attribute.String("coord.service", service),
		attribute.String("coord.namespace", r.ns),
	)
	if err := r.client.CoreV1().ConfigMaps(r.ns).Delete(ctx, service, metav1.DeleteOptions{}); err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/k8s_configmap deregister failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/k8s_configmap deregister done", logx.String("service", service))
	return nil
}
func (r *registerImpl) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := r.client.CoreV1().ConfigMaps(r.ns).List(ctx, metav1.ListOptions{Limit: 1})
	return err
}
func (r *registerImpl) Close() error { return nil }

func init() {
	coordx.RegisterCreator("k8s_configmap", func(cfg *coordx.Config) (coordx.Register, error) {
		r, err := NewRegister(cfg)
		if err != nil {
			return nil, err
		}
		return r, nil
	})
}
