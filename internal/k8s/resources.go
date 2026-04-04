package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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
	CPUMillis       int64  // current usage in millicores (-1 = unavailable)
	MemBytes        int64  // current usage in bytes (-1 = unavailable)
}

func (p PodResource) Name() string { return p.PodName }

func (p PodResource) TableHeaders() []string {
	return []string{"NAME", "STATUS", "READY", "RESTARTS", "CPU", "MEMORY", "AGE"}
}

func (p PodResource) TableRow() []string {
	cpu := "-"
	mem := "-"
	if p.CPUMillis >= 0 {
		cpu = fmt.Sprintf("%dm", p.CPUMillis)
	}
	if p.MemBytes >= 0 {
		mem = formatMemory(p.MemBytes)
	}
	return []string{
		p.PodName,
		p.Status,
		fmt.Sprintf("%d/%d", p.ReadyContainers, p.TotalContainers),
		fmt.Sprintf("%d", p.Restarts),
		cpu,
		mem,
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
	cpu := "-"
	mem := "-"
	if p.CPUMillis >= 0 {
		cpu = fmt.Sprintf("%dm", p.CPUMillis)
	}
	if p.MemBytes >= 0 {
		mem = formatMemory(p.MemBytes)
	}
	return []DetailRow{
		{"Name", p.PodName},
		{"Namespace", p.Namespace},
		{"Status", p.Status},
		{"Ready", fmt.Sprintf("%d/%d", p.ReadyContainers, p.TotalContainers)},
		{"Node", p.Node},
		{"IP", p.IP},
		{"CPU", cpu},
		{"Memory", mem},
		{"Restarts", fmt.Sprintf("%d", p.Restarts)},
		{"Containers", strings.Join(p.Containers, ", ")},
		{"Age", p.Age},
	}
}

// --- Deployment ---

type DeploymentResource struct {
	DepName         string
	Namespace       string
	ReadyReplicas   int32
	DesiredReplicas int32
	UpToDate        int32
	Available       int32
	Strategy        string
	Containers      []string
	Images          []string
	Age             string
}

func (d DeploymentResource) Name() string { return d.DepName }

func (d DeploymentResource) TableHeaders() []string {
	return []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"}
}

func (d DeploymentResource) TableRow() []string {
	return []string{
		d.DepName,
		fmt.Sprintf("%d/%d", d.ReadyReplicas, d.DesiredReplicas),
		fmt.Sprintf("%d", d.UpToDate),
		fmt.Sprintf("%d", d.Available),
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

// --- StatefulSet ---

type StatefulSetResource struct {
	StsName         string
	Namespace       string
	ReadyReplicas   int32
	DesiredReplicas int32
	Images          []string
	Age             string
}

func (s StatefulSetResource) Name() string { return s.StsName }

func (s StatefulSetResource) TableHeaders() []string {
	return []string{"NAME", "READY", "AGE"}
}

func (s StatefulSetResource) TableRow() []string {
	return []string{
		s.StsName,
		fmt.Sprintf("%d/%d", s.ReadyReplicas, s.DesiredReplicas),
		s.Age,
	}
}

func (s StatefulSetResource) Health() HealthStatus {
	switch {
	case s.ReadyReplicas == s.DesiredReplicas && s.DesiredReplicas > 0:
		return HealthOK
	case s.ReadyReplicas == 0 && s.DesiredReplicas > 0:
		return HealthError
	case s.ReadyReplicas < s.DesiredReplicas:
		return HealthWarning
	default:
		return HealthNeutral
	}
}

func (s StatefulSetResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", s.StsName},
		{"Namespace", s.Namespace},
		{"Replicas", fmt.Sprintf("%d/%d", s.ReadyReplicas, s.DesiredReplicas)},
		{"Images", strings.Join(s.Images, ", ")},
		{"Age", s.Age},
	}
}

// --- DaemonSet ---

type DaemonSetResource struct {
	DsName          string
	Namespace       string
	Desired         int32
	Ready           int32
	NodeSelector    string
	Images          []string
	Age             string
}

func (d DaemonSetResource) Name() string { return d.DsName }

func (d DaemonSetResource) TableHeaders() []string {
	return []string{"NAME", "DESIRED", "READY", "AGE"}
}

func (d DaemonSetResource) TableRow() []string {
	return []string{
		d.DsName,
		fmt.Sprintf("%d", d.Desired),
		fmt.Sprintf("%d", d.Ready),
		d.Age,
	}
}

func (d DaemonSetResource) Health() HealthStatus {
	switch {
	case d.Ready == d.Desired && d.Desired > 0:
		return HealthOK
	case d.Ready == 0 && d.Desired > 0:
		return HealthError
	case d.Ready < d.Desired:
		return HealthWarning
	default:
		return HealthNeutral
	}
}

func (d DaemonSetResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", d.DsName},
		{"Namespace", d.Namespace},
		{"Desired", fmt.Sprintf("%d", d.Desired)},
		{"Ready", fmt.Sprintf("%d", d.Ready)},
		{"Node Selector", d.NodeSelector},
		{"Images", strings.Join(d.Images, ", ")},
		{"Age", d.Age},
	}
}

// --- Job ---

type JobResource struct {
	JobName     string
	Namespace   string
	Completions string // "1/3" format
	Duration    string
	Status      string // Complete, Running, Failed
	Age         string
}

func (j JobResource) Name() string { return j.JobName }

func (j JobResource) TableHeaders() []string {
	return []string{"NAME", "COMPLETIONS", "DURATION", "STATUS", "AGE"}
}

func (j JobResource) TableRow() []string {
	return []string{j.JobName, j.Completions, j.Duration, j.Status, j.Age}
}

func (j JobResource) Health() HealthStatus {
	switch j.Status {
	case "Complete":
		return HealthOK
	case "Running":
		return HealthNeutral
	case "Failed":
		return HealthError
	default:
		return HealthNeutral
	}
}

func (j JobResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", j.JobName},
		{"Namespace", j.Namespace},
		{"Completions", j.Completions},
		{"Duration", j.Duration},
		{"Status", j.Status},
		{"Age", j.Age},
	}
}

// --- CronJob ---

type CronJobResource struct {
	CjName       string
	Namespace    string
	Schedule     string
	Suspend      bool
	Active       int
	LastSchedule string
	Age          string
}

func (cj CronJobResource) Name() string { return cj.CjName }

func (cj CronJobResource) TableHeaders() []string {
	return []string{"NAME", "SCHEDULE", "SUSPEND", "ACTIVE", "LAST SCHEDULE", "AGE"}
}

func (cj CronJobResource) TableRow() []string {
	return []string{
		cj.CjName,
		cj.Schedule,
		fmt.Sprintf("%v", cj.Suspend),
		fmt.Sprintf("%d", cj.Active),
		cj.LastSchedule,
		cj.Age,
	}
}

func (cj CronJobResource) Health() HealthStatus { return HealthNeutral }

func (cj CronJobResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", cj.CjName},
		{"Namespace", cj.Namespace},
		{"Schedule", cj.Schedule},
		{"Suspend", fmt.Sprintf("%v", cj.Suspend)},
		{"Active", fmt.Sprintf("%d", cj.Active)},
		{"Last Schedule", cj.LastSchedule},
		{"Age", cj.Age},
	}
}

// --- Ingress ---

type IngressResource struct {
	IngName   string
	Namespace string
	Class     string
	Hosts     string
	Ports     string
	Age       string
}

func (i IngressResource) Name() string { return i.IngName }

func (i IngressResource) TableHeaders() []string {
	return []string{"NAME", "CLASS", "HOSTS", "PORTS", "AGE"}
}

func (i IngressResource) TableRow() []string {
	return []string{i.IngName, i.Class, i.Hosts, i.Ports, i.Age}
}

func (i IngressResource) Health() HealthStatus { return HealthNeutral }

func (i IngressResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", i.IngName},
		{"Namespace", i.Namespace},
		{"Class", i.Class},
		{"Hosts", i.Hosts},
		{"Ports", i.Ports},
		{"Age", i.Age},
	}
}

// --- Secret ---

type SecretResource struct {
	SecName   string
	Namespace string
	Type      string
	DataCount int
	Age       string
}

func (s SecretResource) Name() string { return s.SecName }

func (s SecretResource) TableHeaders() []string {
	return []string{"NAME", "TYPE", "DATA", "AGE"}
}

func (s SecretResource) TableRow() []string {
	return []string{s.SecName, s.Type, fmt.Sprintf("%d", s.DataCount), s.Age}
}

func (s SecretResource) Health() HealthStatus { return HealthNeutral }

func (s SecretResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", s.SecName},
		{"Namespace", s.Namespace},
		{"Type", s.Type},
		{"Data Count", fmt.Sprintf("%d", s.DataCount)},
		{"Age", s.Age},
	}
}

// --- PersistentVolumeClaim ---

type PVCResource struct {
	PvcName      string
	Namespace    string
	Status       string
	Volume       string
	Capacity     string
	AccessModes  string
	StorageClass string
	Age          string
}

func (p PVCResource) Name() string { return p.PvcName }

func (p PVCResource) TableHeaders() []string {
	return []string{"NAME", "STATUS", "VOLUME", "CAPACITY", "ACCESS MODES", "AGE"}
}

func (p PVCResource) TableRow() []string {
	return []string{p.PvcName, p.Status, p.Volume, p.Capacity, p.AccessModes, p.Age}
}

func (p PVCResource) Health() HealthStatus {
	switch p.Status {
	case "Bound":
		return HealthOK
	case "Pending":
		return HealthWarning
	case "Lost":
		return HealthError
	default:
		return HealthNeutral
	}
}

func (p PVCResource) Details() []DetailRow {
	return []DetailRow{
		{"Name", p.PvcName},
		{"Namespace", p.Namespace},
		{"Status", p.Status},
		{"Volume", p.Volume},
		{"Capacity", p.Capacity},
		{"Access Modes", p.AccessModes},
		{"Storage Class", p.StorageClass},
		{"Age", p.Age},
	}
}

// --- Event ---

type EventResource struct {
	EventName  string
	Namespace  string
	Type       string // Normal or Warning
	Reason     string
	Object     string // e.g. "Pod/my-app-abc123"
	Message    string
	Count      int32
	LastSeen   string
	FirstSeen  string
	Source     string
}

func (e EventResource) Name() string { return e.EventName }

func (e EventResource) TableHeaders() []string {
	return []string{"TYPE", "REASON", "OBJECT", "COUNT", "LAST SEEN", "MESSAGE"}
}

func (e EventResource) TableRow() []string {
	// Truncate message for the table — the full message lives in Details.
	msg := e.Message
	if len(msg) > 60 {
		msg = msg[:57] + "..."
	}
	return []string{
		e.Type,
		e.Reason,
		e.Object,
		fmt.Sprintf("%d", e.Count),
		e.LastSeen,
		msg,
	}
}

func (e EventResource) Health() HealthStatus {
	switch e.Type {
	case "Warning":
		return HealthWarning
	case "Normal":
		return HealthOK
	default:
		return HealthNeutral
	}
}

func (e EventResource) Details() []DetailRow {
	return []DetailRow{
		{"Type", e.Type},
		{"Reason", e.Reason},
		{"Object", e.Object},
		{"Source", e.Source},
		{"Count", fmt.Sprintf("%d", e.Count)},
		{"First Seen", e.FirstSeen},
		{"Last Seen", e.LastSeen},
		{"Message", e.Message},
	}
}

// --- List methods ---

func formatAge(created metav1.Time) string {
	return fmt.Sprintf("%s", metav1.Now().Sub(created.Time).Round(1_000_000_000))
}

// formatMemory converts bytes to a human-readable format (Ki, Mi, Gi).
func formatMemory(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGi", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.0fMi", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0fKi", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func (c *Client) ListEventResources(ctx context.Context, namespace string) ([]Resource, error) {
	evList, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing events in %s: %w", namespace, err)
	}

	// Sort most-recent-first. EventTime (newer API) or LastTimestamp —
	// events use either depending on which controller emitted them.
	eventTs := func(i int) time.Time {
		e := evList.Items[i]
		if !e.EventTime.IsZero() {
			return e.EventTime.Time
		}
		if !e.LastTimestamp.IsZero() {
			return e.LastTimestamp.Time
		}
		return e.CreationTimestamp.Time
	}
	sort.Slice(evList.Items, func(i, j int) bool {
		return eventTs(i).After(eventTs(j))
	})

	resources := make([]Resource, 0, len(evList.Items))
	for _, ev := range evList.Items {
		obj := fmt.Sprintf("%s/%s", ev.InvolvedObject.Kind, ev.InvolvedObject.Name)

		// Prefer LastTimestamp; fall back to EventTime for newer events
		// that use the v1 Events API, then to CreationTimestamp.
		var lastSeen, firstSeen string
		switch {
		case !ev.LastTimestamp.IsZero():
			lastSeen = formatAge(ev.LastTimestamp)
		case !ev.EventTime.IsZero():
			lastSeen = time.Since(ev.EventTime.Time).Round(time.Second).String()
		default:
			lastSeen = formatAge(ev.CreationTimestamp)
		}
		if !ev.FirstTimestamp.IsZero() {
			firstSeen = formatAge(ev.FirstTimestamp)
		} else {
			firstSeen = lastSeen
		}

		count := ev.Count
		if count == 0 {
			count = 1
		}

		source := ev.Source.Component
		if source == "" && ev.ReportingController != "" {
			source = ev.ReportingController
		}

		resources = append(resources, EventResource{
			EventName: ev.Name,
			Namespace: ev.Namespace,
			Type:      ev.Type,
			Reason:    ev.Reason,
			Object:    obj,
			Message:   ev.Message,
			Count:     count,
			LastSeen:  lastSeen,
			FirstSeen: firstSeen,
			Source:    source,
		})
	}

	return resources, nil
}

func (c *Client) ListPodResources(ctx context.Context, namespace string) ([]Resource, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in %s: %w", namespace, err)
	}

	// Fetch metrics in one batch call — returns an empty map if unavailable.
	metricsMap := c.PodMetricsMap(ctx, namespace)

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

		// Merge metrics — use -1 to signal "unavailable"
		var cpuMillis, memBytes int64 = -1, -1
		if m, ok := metricsMap[pod.Name]; ok {
			cpuMillis = m[0]
			memBytes = m[1]
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
			CPUMillis:       cpuMillis,
			MemBytes:        memBytes,
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
			UpToDate:        dep.Status.UpdatedReplicas,
			Available:       dep.Status.AvailableReplicas,
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

func (c *Client) ListStatefulSetResources(ctx context.Context, namespace string) ([]Resource, error) {
	stsList, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing statefulsets in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(stsList.Items))
	for _, sts := range stsList.Items {
		var desired int32
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		images := make([]string, 0, len(sts.Spec.Template.Spec.Containers))
		for _, c := range sts.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		resources = append(resources, StatefulSetResource{
			StsName:         sts.Name,
			Namespace:       sts.Namespace,
			ReadyReplicas:   sts.Status.ReadyReplicas,
			DesiredReplicas: desired,
			Images:          images,
			Age:             formatAge(sts.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListDaemonSetResources(ctx context.Context, namespace string) ([]Resource, error) {
	dsList, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing daemonsets in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(dsList.Items))
	for _, ds := range dsList.Items {
		nodeSelector := "<none>"
		if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
			var parts []string
			for k, v := range ds.Spec.Template.Spec.NodeSelector {
				parts = append(parts, k+"="+v)
			}
			nodeSelector = strings.Join(parts, ", ")
		}
		images := make([]string, 0, len(ds.Spec.Template.Spec.Containers))
		for _, c := range ds.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		resources = append(resources, DaemonSetResource{
			DsName:       ds.Name,
			Namespace:    ds.Namespace,
			Desired:      ds.Status.DesiredNumberScheduled,
			Ready:        ds.Status.NumberReady,
			NodeSelector: nodeSelector,
			Images:       images,
			Age:          formatAge(ds.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListJobResources(ctx context.Context, namespace string) ([]Resource, error) {
	jobList, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing jobs in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(jobList.Items))
	for _, job := range jobList.Items {
		var desired int32 = 1
		if job.Spec.Completions != nil {
			desired = *job.Spec.Completions
		}

		status := "Running"
		for _, cond := range job.Status.Conditions {
			if cond.Type == "Complete" && cond.Status == "True" {
				status = "Complete"
			} else if cond.Type == "Failed" && cond.Status == "True" {
				status = "Failed"
			}
		}

		duration := "<running>"
		if job.Status.StartTime != nil {
			end := time.Now()
			if job.Status.CompletionTime != nil {
				end = job.Status.CompletionTime.Time
			}
			duration = end.Sub(job.Status.StartTime.Time).Round(time.Second).String()
		}

		resources = append(resources, JobResource{
			JobName:     job.Name,
			Namespace:   job.Namespace,
			Completions: fmt.Sprintf("%d/%d", job.Status.Succeeded, desired),
			Duration:    duration,
			Status:      status,
			Age:         formatAge(job.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListCronJobResources(ctx context.Context, namespace string) ([]Resource, error) {
	cjList, err := c.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing cronjobs in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(cjList.Items))
	for _, cj := range cjList.Items {
		suspend := false
		if cj.Spec.Suspend != nil {
			suspend = *cj.Spec.Suspend
		}

		lastSchedule := "<none>"
		if cj.Status.LastScheduleTime != nil {
			lastSchedule = formatAge(*cj.Status.LastScheduleTime)
		}

		resources = append(resources, CronJobResource{
			CjName:       cj.Name,
			Namespace:    cj.Namespace,
			Schedule:     cj.Spec.Schedule,
			Suspend:      suspend,
			Active:       len(cj.Status.Active),
			LastSchedule: lastSchedule,
			Age:          formatAge(cj.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListIngressResources(ctx context.Context, namespace string) ([]Resource, error) {
	ingList, err := c.clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing ingresses in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(ingList.Items))
	for _, ing := range ingList.Items {
		class := "<none>"
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}

		var hosts []string
		var ports []string
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				hosts = append(hosts, rule.Host)
			}
		}
		if len(ing.Spec.TLS) > 0 {
			ports = append(ports, "443")
		}
		ports = append(ports, "80")

		hostStr := "*"
		if len(hosts) > 0 {
			hostStr = strings.Join(hosts, ", ")
		}

		resources = append(resources, IngressResource{
			IngName:   ing.Name,
			Namespace: ing.Namespace,
			Class:     class,
			Hosts:     hostStr,
			Ports:     strings.Join(ports, ", "),
			Age:       formatAge(ing.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListSecretResources(ctx context.Context, namespace string) ([]Resource, error) {
	secList, err := c.clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing secrets in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(secList.Items))
	for _, sec := range secList.Items {
		resources = append(resources, SecretResource{
			SecName:   sec.Name,
			Namespace: sec.Namespace,
			Type:      string(sec.Type),
			DataCount: len(sec.Data),
			Age:       formatAge(sec.CreationTimestamp),
		})
	}

	return resources, nil
}

func (c *Client) ListPVCResources(ctx context.Context, namespace string) ([]Resource, error) {
	pvcList, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pvcs in %s: %w", namespace, err)
	}

	resources := make([]Resource, 0, len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		capacity := "<pending>"
		if qty, ok := pvc.Status.Capacity["storage"]; ok {
			capacity = qty.String()
		}

		var accessModes []string
		for _, am := range pvc.Spec.AccessModes {
			accessModes = append(accessModes, string(am))
		}

		storageClass := "<default>"
		if pvc.Spec.StorageClassName != nil {
			storageClass = *pvc.Spec.StorageClassName
		}

		resources = append(resources, PVCResource{
			PvcName:      pvc.Name,
			Namespace:    pvc.Namespace,
			Status:       string(pvc.Status.Phase),
			Volume:       pvc.Spec.VolumeName,
			Capacity:     capacity,
			AccessModes:  strings.Join(accessModes, ", "),
			StorageClass: storageClass,
			Age:          formatAge(pvc.CreationTimestamp),
		})
	}

	return resources, nil
}
