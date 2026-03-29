package k8s

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthStatus represents the health of a resource for color coding.
type HealthStatus int

const (
	HealthOK      HealthStatus = iota // everything is fine — green
	HealthWarning                     // degraded but not dead — yellow
	HealthError                       // failing — red
	HealthNeutral                     // not applicable (e.g., configmaps) — default color
)

// Resource is an interface that all Kubernetes resource types implement.
// We've added TableHeaders/TableRow for the columnar list view,
// and Health for color coding.
type Resource interface {
	Name() string
	Details() []DetailRow

	// TableHeaders returns column names for the list view.
	// Every concrete type defines its own columns — pods show status/ready/restarts,
	// deployments show replicas, etc.
	TableHeaders() []string

	// TableRow returns the values for each column, matching TableHeaders order.
	TableRow() []string

	// Health returns the health status for color coding the row.
	Health() HealthStatus
}

type DetailRow struct {
	Key   string
	Value string
}

// --- Pod ---

type PodResource struct {
	PodName         string
	Namespace       string
	Status          string
	Node            string
	IP              string
	Restarts        int32
	Containers      []string
	ReadyContainers int
	TotalContainers int
	Age             string
}

func (p PodResource) Name() string { return p.PodName }

func (p PodResource) TableHeaders() []string {
	return []string{"NAME", "STATUS", "READY", "RESTARTS", "AGE"}
}

func (p PodResource) TableRow() []string {
	return []string{
		p.PodName,
		p.Status,
		fmt.Sprintf("%d/%d", p.ReadyContainers, p.TotalContainers),
		fmt.Sprintf("%d", p.Restarts),
		p.Age,
	}
}

func (p PodResource) Health() HealthStatus {
	switch {
	case p.Status == "Running" && p.ReadyContainers == p.TotalContainers:
		return HealthOK
	case p.Status == "Succeeded":
		return HealthOK
	case p.Status == "Pending":
		return HealthWarning
	case p.Status == "Failed" || p.Status == "CrashLoopBackOff" || p.Status == "Error":
		return HealthError
	case p.ReadyContainers < p.TotalContainers:
		return HealthWarning
	default:
		return HealthNeutral
	}
}

func (p PodResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", p.PodName},
		{"Namespace", p.Namespace},
		{"Status", p.Status},
		{"Ready", fmt.Sprintf("%d/%d", p.ReadyContainers, p.TotalContainers)},
		{"Node", p.Node},
		{"IP", p.IP},
		{"Restarts", fmt.Sprintf("%d", p.Restarts)},
		{"Containers", strings.Join(p.Containers, ", ")},
		{"Age", p.Age},
	}
}

// --- Deployment ---

type DeploymentResource struct {
	DepName        string
	Namespace      string
	ReadyReplicas  int32
	DesiredReplicas int32
	Strategy       string
	Containers     []string
	Images         []string
	Age            string
}

func (d DeploymentResource) Name() string { return d.DepName }

func (d DeploymentResource) TableHeaders() []string {
	return []string{"NAME", "READY", "STRATEGY", "AGE"}
}

func (d DeploymentResource) TableRow() []string {
	return []string{
		d.DepName,
		fmt.Sprintf("%d/%d", d.ReadyReplicas, d.DesiredReplicas),
		d.Strategy,
		d.Age,
	}
}

func (d DeploymentResource) Health() HealthStatus {
	switch {
	case d.ReadyReplicas == d.DesiredReplicas && d.DesiredReplicas > 0:
		return HealthOK
	case d.ReadyReplicas == 0 && d.DesiredReplicas > 0:
		return HealthError
	case d.ReadyReplicas < d.DesiredReplicas:
		return HealthWarning
	default:
		return HealthNeutral
	}
}

func (d DeploymentResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", d.DepName},
		{"Namespace", d.Namespace},
		{"Replicas", fmt.Sprintf("%d/%d", d.ReadyReplicas, d.DesiredReplicas)},
		{"Strategy", d.Strategy},
		{"Containers", strings.Join(d.Containers, ", ")},
		{"Images", strings.Join(d.Images, ", ")},
		{"Age", d.Age},
	}
}

// --- Service ---

type ServiceResource struct {
	SvcName    string
	Namespace  string
	Type       string
	ClusterIP  string
	ExternalIP string
	Ports      string
	Age        string
}

func (s ServiceResource) Name() string { return s.SvcName }

func (s ServiceResource) TableHeaders() []string {
	return []string{"NAME", "TYPE", "CLUSTER-IP", "PORTS", "AGE"}
}

func (s ServiceResource) TableRow() []string {
	return []string{
		s.SvcName,
		s.Type,
		s.ClusterIP,
		s.Ports,
		s.Age,
	}
}

func (s ServiceResource) Health() HealthStatus { return HealthNeutral }

func (s ServiceResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", s.SvcName},
		{"Namespace", s.Namespace},
		{"Type", s.Type},
		{"Cluster IP", s.ClusterIP},
		{"External IP", s.ExternalIP},
		{"Ports", s.Ports},
		{"Age", s.Age},
	}
}

// --- ConfigMap ---

type ConfigMapResource struct {
	CmName    string
	Namespace string
	DataKeys  []string
	DataCount int
	Age       string
}

func (cm ConfigMapResource) Name() string { return cm.CmName }

func (cm ConfigMapResource) TableHeaders() []string {
	return []string{"NAME", "DATA", "AGE"}
}

func (cm ConfigMapResource) TableRow() []string {
	return []string{
		cm.CmName,
		fmt.Sprintf("%d", cm.DataCount),
		cm.Age,
	}
}

func (cm ConfigMapResource) Health() HealthStatus { return HealthNeutral }

func (cm ConfigMapResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", cm.CmName},
		{"Namespace", cm.Namespace},
		{"Data Keys", strings.Join(cm.DataKeys, ", ")},
		{"Data Count", fmt.Sprintf("%d", cm.DataCount)},
		{"Age", cm.Age},
	}
}

// --- List methods ---

func formatAge(created metav1.Time) string {
	return fmt.Sprintf("%s", metav1.Now().Sub(created.Time).Round(1_000_000_000))
}

func (c *Client) ListPodResources(ctx context.Context, namespace string) ([]Resource, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(podList.Items))
	for _, pod := range podList.Items {
		var restarts int32
		var readyCount int
		containers := make([]string, 0, len(pod.Spec.Containers))
		for _, c := range pod.Spec.Containers {
			containers = append(containers, c.Name)
		}
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
			if cs.Ready {
				readyCount++
			}
		}

		resources = append(resources, PodResource{
			PodName:         pod.Name,
			Namespace:       pod.Namespace,
			Status:          string(pod.Status.Phase),
			Node:            pod.Spec.NodeName,
			IP:              pod.Status.PodIP,
			Restarts:        restarts,
			Containers:      containers,
			ReadyContainers: readyCount,
			TotalContainers: len(pod.Spec.Containers),
			Age:             formatAge(pod.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListDeploymentResources(ctx context.Context, namespace string) ([]Resource, error) {
	depList, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(depList.Items))
	for _, dep := range depList.Items {
		containers := make([]string, 0, len(dep.Spec.Template.Spec.Containers))
		images := make([]string, 0, len(dep.Spec.Template.Spec.Containers))
		for _, c := range dep.Spec.Template.Spec.Containers {
			containers = append(containers, c.Name)
			images = append(images, c.Image)
		}

		var desired int32
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}

		resources = append(resources, DeploymentResource{
			DepName:         dep.Name,
			Namespace:       dep.Namespace,
			ReadyReplicas:   dep.Status.ReadyReplicas,
			DesiredReplicas: desired,
			Strategy:        string(dep.Spec.Strategy.Type),
			Containers:      containers,
			Images:          images,
			Age:             formatAge(dep.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListServiceResources(ctx context.Context, namespace string) ([]Resource, error) {
	svcList, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(svcList.Items))
	for _, svc := range svcList.Items {
		var ports []string
		for _, p := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}

		externalIP := "<none>"
		if len(svc.Spec.ExternalIPs) > 0 {
			externalIP = strings.Join(svc.Spec.ExternalIPs, ", ")
		} else if svc.Spec.Type == "LoadBalancer" && len(svc.Status.LoadBalancer.Ingress) > 0 {
			externalIP = svc.Status.LoadBalancer.Ingress[0].IP
		}

		resources = append(resources, ServiceResource{
			SvcName:    svc.Name,
			Namespace:  svc.Namespace,
			Type:       string(svc.Spec.Type),
			ClusterIP:  svc.Spec.ClusterIP,
			ExternalIP: externalIP,
			Ports:      strings.Join(ports, ", "),
			Age:        formatAge(svc.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListConfigMapResources(ctx context.Context, namespace string) ([]Resource, error) {
	cmList, err := c.clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing configmaps in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(cmList.Items))
	for _, cm := range cmList.Items {
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}

		resources = append(resources, ConfigMapResource{
			CmName:    cm.Name,
			Namespace: cm.Namespace,
			DataKeys:  keys,
			DataCount: len(cm.Data),
			Age:       formatAge(cm.CreationTimestamp),
		})
	}

	return resources, nil
}
