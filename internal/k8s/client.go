package k8s

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/yaml"
)

// ClusterStats holds cluster-wide resource utilization.
type ClusterStats struct {
	CPUUsedMillis     int64
	CPUTotalMillis    int64
	MemUsedBytes      int64
	MemTotalBytes     int64
	NodeCount         int
	PodCount          int
	MetricsAvailable  bool
}

// Client wraps the Kubernetes clientset and optional metrics client.
type Client struct {
	clientset     *kubernetes.Clientset
	metricsClient metricsv.Interface // nil if metrics-server is unavailable
	ContextName   string
	ClusterName   string
}

// ContextInfo holds metadata about a kubeconfig context.
type ContextInfo struct {
	Name    string
	Cluster string
	File    string // absolute path to the kubeconfig file this context came from
}

// discoverKubeconfigs finds all kubeconfig files by checking:
//  1. The KUBECONFIG environment variable (colon-separated paths, the standard convention)
//  2. The default ~/.kube/config
//  3. Any .yaml, .yml, or .conf files in ~/.kube/
//
// This lets users drop additional kubeconfig files into ~/.kube/ and have them
// automatically discovered — no need to manually edit KUBECONFIG.
func discoverKubeconfigs() []string {
	home := homedir.HomeDir()
	if home == "" {
		return nil
	}

	kubeDir := filepath.Join(home, ".kube")
	seen := make(map[string]bool)
	var files []string

	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if seen[abs] {
			return
		}
		// Only add files that actually exist
		if _, err := os.Stat(abs); err != nil {
			return
		}
		seen[abs] = true
		files = append(files, abs)
	}

	// 1. KUBECONFIG env var — filepath.SplitList handles the OS-specific
	//    separator (colon on unix, semicolon on windows).
	if env := os.Getenv("KUBECONFIG"); env != "" {
		for _, p := range filepath.SplitList(env) {
			add(p)
		}
	}

	// 2. Default config file
	add(filepath.Join(kubeDir, "config"))

	// 3. Scan ~/.kube/ for additional kubeconfig files
	entries, err := os.ReadDir(kubeDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			ext := filepath.Ext(name)
			if ext == ".yaml" || ext == ".yml" || ext == ".conf" {
				add(filepath.Join(kubeDir, name))
			}
		}
	}

	return files
}

// ListContexts discovers all kubeconfig files and returns every context
// found across all of them. Each ContextInfo records which file it came
// from so the UI can display the source and connect with the right file.
func ListContexts() []ContextInfo {
	files := discoverKubeconfigs()

	var contexts []ContextInfo
	seen := make(map[string]bool)

	for _, file := range files {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: file}
		kubeLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, &clientcmd.ConfigOverrides{},
		)

		rawConfig, err := kubeLoader.RawConfig()
		if err != nil {
			continue // skip files that aren't valid kubeconfigs
		}

		for name, ctx := range rawConfig.Contexts {
			if seen[name] {
				continue // first file wins for duplicate context names
			}
			seen[name] = true
			contexts = append(contexts, ContextInfo{
				Name:    name,
				Cluster: ctx.Cluster,
				File:    file,
			})
		}
	}

	// Sort by file then name so contexts from the same kubeconfig are grouped
	sort.Slice(contexts, func(i, j int) bool {
		if contexts[i].File != contexts[j].File {
			return contexts[i].File < contexts[j].File
		}
		return contexts[i].Name < contexts[j].Name
	})

	return contexts
}

// NewClientForContext creates a connection using a specific kubeconfig context
// from the given kubeconfig file.
func NewClientForContext(contextName, kubeconfigPath string) (*Client, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	kubeLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	rawConfig, err := kubeLoader.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	clusterName := ""
	if ctx, ok := rawConfig.Contexts[contextName]; ok {
		clusterName = ctx.Cluster
	}

	config, err := kubeLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building config for context %s: %w", contextName, err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	mc, _ := metricsv.NewForConfig(config)

	return &Client{
		clientset:     clientset,
		metricsClient: mc,
		ContextName:   contextName,
		ClusterName:   clusterName,
	}, nil
}

// NewClient creates a connection to Kubernetes using your ~/.kube/config.
//
// This function returns (*Client, error) — the "multi-return" pattern is
// THE way Go handles errors. No exceptions, no try/catch. Every function
// that can fail returns an error as its last value, and the caller decides
// what to do with it. You'll write this pattern hundreds of times.
func NewClient() (*Client, error) {
	// homedir.HomeDir() gets the user's home directory in a cross-platform way.
	home := homedir.HomeDir()
	if home == "" {
		return nil, fmt.Errorf("could not find home directory")
	}

	kubeconfig := filepath.Join(home, ".kube", "config")

	// Load the full kubeconfig to extract context/cluster metadata.
	// clientcmd.NewNonInteractiveDeferredLoadingClientConfig is a mouthful,
	// but it's the standard way to load kubeconfig with all its context info.
	// Go libraries sometimes have long names — the convention is clarity over brevity.
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	// RawConfig gives us the full kubeconfig structure — contexts, clusters, users.
	rawConfig, err := kubeLoader.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	contextName := rawConfig.CurrentContext
	clusterName := ""
	if ctx, ok := rawConfig.Contexts[contextName]; ok {
		clusterName = ctx.Cluster
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	// Create metrics client — this is optional. If metrics-server isn't
	// installed on the cluster, the client will be created successfully
	// but API calls will fail. We handle that gracefully at call sites.
	mc, _ := metricsv.NewForConfig(config)

	return &Client{
		clientset:     clientset,
		metricsClient: mc,
		ContextName:   contextName,
		ClusterName:   clusterName,
	}, nil
}

// ListNamespaces returns the names of all namespaces in the cluster.
//
// context.Context is Go's way of handling cancellation and timeouts.
// Almost every function that does I/O (network calls, file reads, DB queries)
// takes a context as its first parameter. This lets callers say "cancel this
// if it takes too long" or "stop if the user quit the app".
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	// c.clientset.CoreV1() gives us the "core/v1" API group — that's where
	// namespaces, pods, services, and other fundamental resources live.
	// Kubernetes groups its APIs by version: CoreV1, AppsV1, NetworkingV1, etc.
	nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	// We allocate the slice with make([]string, 0, len(nsList.Items)).
	// The third argument is the "capacity" — it pre-allocates memory.
	// Since we know exactly how many items we'll have, this avoids
	// repeated memory allocations as the slice grows during append.
	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}

	return names, nil
}

// GetClusterStats returns cluster-wide CPU and memory utilization.
// If metrics-server is not available, it returns stats with MetricsAvailable=false
// but still includes node/pod counts from the core API.
func (c *Client) GetClusterStats(ctx context.Context) ClusterStats {
	stats := ClusterStats{}

	// Node count + allocatable capacity (always available)
	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		stats.NodeCount = len(nodeList.Items)
		for _, node := range nodeList.Items {
			stats.CPUTotalMillis += node.Status.Allocatable.Cpu().MilliValue()
			stats.MemTotalBytes += node.Status.Allocatable.Memory().Value()
		}
	}

	// Pod count (always available)
	podList, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		stats.PodCount = len(podList.Items)
	}

	// Node metrics (requires metrics-server)
	if c.metricsClient != nil {
		nodeMetrics, err := c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err == nil && len(nodeMetrics.Items) > 0 {
			stats.MetricsAvailable = true
			for _, nm := range nodeMetrics.Items {
				stats.CPUUsedMillis += nm.Usage.Cpu().MilliValue()
				stats.MemUsedBytes += nm.Usage.Memory().Value()
			}
		}
	}

	return stats
}

// PodMetricsMap returns a map of pod name → (cpuMillis, memBytes) for a namespace.
// Returns an empty map if metrics-server is unavailable (graceful degradation).
func (c *Client) PodMetricsMap(ctx context.Context, namespace string) map[string][2]int64 {
	result := make(map[string][2]int64)
	if c.metricsClient == nil {
		return result
	}

	podMetrics, err := c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return result
	}

	for _, pm := range podMetrics.Items {
		var cpuTotal, memTotal int64
		for _, container := range pm.Containers {
			cpuTotal += container.Usage.Cpu().MilliValue()
			memTotal += container.Usage.Memory().Value()
		}
		result[pm.Name] = [2]int64{cpuTotal, memTotal}
	}

	return result
}

// StreamPodLogs streams log lines from a pod into a channel.
//
// This introduces several important Go concepts:
//
// 1. CHANNELS (chan string): Go's way of communicating between goroutines.
//    A channel is like a thread-safe pipe — one side writes, the other reads.
//    We return a read-only channel (<-chan) so the caller can only receive, not send.
//
// 2. GOROUTINES (go func()): Lightweight concurrent functions. The "go" keyword
//    launches a function in the background. Unlike OS threads, goroutines are
//    managed by the Go runtime — you can spawn thousands cheaply.
//
// 3. CONTEXT CANCELLATION: The ctx parameter lets the caller stop the stream.
//    When they call cancel(), the Kubernetes API closes the connection,
//    the io.Reader returns an error, and our goroutine cleans up.
//
// 4. io.Reader: Go's universal interface for reading bytes. The log stream,
//    files, HTTP responses, network connections — they all implement io.Reader.
//    bufio.Scanner wraps a Reader to read line-by-line.
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName string) (<-chan string, error) {
	// Set up the log request. Follow=true means "keep streaming new lines"
	// (like kubectl logs -f). TailLines shows the last N lines first so you
	// get some history before the live stream starts.
	tailLines := int64(50)
	opts := &corev1.PodLogOptions{
		Follow:    true,
		TailLines: &tailLines,
	}

	// GetLogs returns a *rest.Request, and .Stream() executes it,
	// giving us an io.ReadCloser — a stream we can read from AND close.
	stream, err := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening log stream for %s/%s: %w", namespace, podName, err)
	}

	// Create a channel to send log lines through.
	// The buffer size (100) means up to 100 lines can be queued before
	// the sender blocks. This prevents the goroutine from stalling if
	// the UI is briefly slow to consume lines.
	lines := make(chan string, 100)

	// Launch a goroutine to read from the stream and send lines into the channel.
	// This runs concurrently — the function returns immediately while this
	// keeps reading in the background.
	go func() {
		// defer runs when the function exits, no matter how it exits.
		// This ensures we always clean up, even if there's a panic.
		// Order matters: deferred calls run LIFO (last in, first out).
		defer close(lines) // Signal to the reader that no more lines are coming
		defer stream.Close()

		scanner := bufio.NewScanner(stream)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				// Context was cancelled (user pressed esc). Stop reading.
				return
			case lines <- scanner.Text():
				// Sent the line successfully. scanner.Text() returns the
				// current line WITHOUT the trailing newline.
			}
		}
		// scanner.Scan() returns false when the stream ends or errors.
		// If there's an error, scanner.Err() returns it.
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			// Only send the error if we weren't deliberately cancelled.
			// We reuse the string channel — a production app might use a
			// separate error channel, but this keeps things simple.
			lines <- fmt.Sprintf("[error reading logs: %v]", err)
		}
	}()

	// The caller gets back a channel that log lines will appear on.
	// They range over it or select on it to receive lines as they arrive.
	return lines, nil
}

// GetPodLogs fetches a snapshot of logs (non-streaming).
// Useful for getting a chunk of logs without following.
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName string) (string, error) {
	tailLines := int64(100)
	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}

	// Without Follow, Stream gives us a finite reader that returns all
	// available logs and then EOF.
	stream, err := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("getting logs for %s/%s: %w", namespace, podName, err)
	}
	defer stream.Close()

	// io.ReadAll reads everything from a Reader into a byte slice.
	// Then we convert []byte to string. In Go, strings are just
	// immutable byte slices under the hood — the conversion is cheap.
	bytes, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("reading logs for %s/%s: %w", namespace, podName, err)
	}

	return string(bytes), nil
}

// GetResourceYAML fetches a resource as YAML text. We use the typed API to get
// the resource as JSON, convert it to a map, clean out the noisy server-managed
// fields (managedFields, resourceVersion, uid, etc.), then convert to YAML.
//
// Why JSON→map→YAML instead of just getting YAML directly? The Kubernetes
// client-go library speaks JSON natively. There's no built-in YAML endpoint.
// The "sigs.k8s.io/yaml" package bridges the two — it's the standard way to
// do JSON↔YAML conversion in the Kubernetes ecosystem.
func (c *Client) GetResourceYAML(ctx context.Context, resType, namespace, name string) (string, error) {
	var raw []byte
	var err error

	// Each resource type uses a different API group. This switch is similar
	// to our fetchResources — we dispatch to the right API based on type.
	switch resType {
	case "Pods":
		obj, e := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "Deployments":
		obj, e := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "StatefulSets":
		obj, e := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "DaemonSets":
		obj, e := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "Services":
		obj, e := c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "ConfigMaps":
		obj, e := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "Secrets":
		obj, e := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "Jobs":
		obj, e := c.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "CronJobs":
		obj, e := c.clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "Ingresses":
		obj, e := c.clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "PVCs":
		obj, e := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	case "Events":
		obj, e := c.clientset.CoreV1().Events(namespace).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			err = e
		} else {
			raw, err = json.Marshal(obj)
		}
	default:
		return "", fmt.Errorf("unsupported resource type: %s", resType)
	}

	if err != nil {
		return "", fmt.Errorf("getting %s/%s: %w", resType, name, err)
	}

	// Convert JSON to a map so we can clean up noisy server-managed fields.
	// These fields clutter the YAML and aren't useful for editing.
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("parsing resource: %w", err)
	}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		delete(meta, "managedFields")
		delete(meta, "uid")
		delete(meta, "creationTimestamp")
		delete(meta, "generation")
		delete(meta, "resourceVersion")
	}
	delete(obj, "status")

	// yaml.Marshal from sigs.k8s.io/yaml produces Kubernetes-style YAML.
	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("converting to YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// ApplyYAML takes a YAML string and applies it to the cluster.
// It converts YAML→JSON and uses the strategic merge patch to update
// the resource. This is similar to `kubectl apply` — it merges your
// changes with the existing resource rather than replacing it entirely.
//
// types.StrategicMergePatchType is the "smart" patch type for Kubernetes.
// Unlike a plain merge, it understands Kubernetes-specific merge semantics
// (e.g., it knows how to merge container lists in a pod spec by name
// rather than by array index).
func (c *Client) ApplyYAML(ctx context.Context, resType, namespace, name string, yamlData []byte) error {
	// Convert YAML to JSON — the Kubernetes API speaks JSON.
	jsonData, err := yaml.YAMLToJSON(yamlData)
	if err != nil {
		return fmt.Errorf("converting YAML to JSON: %w", err)
	}

	patchType := types.StrategicMergePatchType

	switch resType {
	case "Pods":
		_, err = c.clientset.CoreV1().Pods(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "Deployments":
		_, err = c.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "StatefulSets":
		_, err = c.clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "DaemonSets":
		_, err = c.clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "Services":
		_, err = c.clientset.CoreV1().Services(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "ConfigMaps":
		_, err = c.clientset.CoreV1().ConfigMaps(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "Secrets":
		_, err = c.clientset.CoreV1().Secrets(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "Jobs":
		_, err = c.clientset.BatchV1().Jobs(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "CronJobs":
		_, err = c.clientset.BatchV1().CronJobs(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "Ingresses":
		_, err = c.clientset.NetworkingV1().Ingresses(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	case "PVCs":
		_, err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Patch(ctx, name, patchType, jsonData, metav1.PatchOptions{})
	default:
		return fmt.Errorf("unsupported resource type: %s", resType)
	}

	return err
}

// DeleteResource deletes a resource by type, namespace, and name.
func (c *Client) DeleteResource(ctx context.Context, resType, namespace, name string) error {
	var err error
	switch resType {
	case "Pods":
		err = c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Deployments":
		err = c.clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "StatefulSets":
		err = c.clientset.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "DaemonSets":
		err = c.clientset.AppsV1().DaemonSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Services":
		err = c.clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "ConfigMaps":
		err = c.clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Secrets":
		err = c.clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Jobs":
		err = c.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "CronJobs":
		err = c.clientset.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "Ingresses":
		err = c.clientset.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "PVCs":
		err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	default:
		return fmt.Errorf("unsupported resource type: %s", resType)
	}
	return err
}

// RolloutRestart triggers a rollout restart for Deployments, StatefulSets, or
// DaemonSets. This works exactly like `kubectl rollout restart` — it patches
// the pod template annotation with the current timestamp, which changes the
// pod spec and triggers a rolling update. No pods are directly deleted;
// the controller handles the gradual replacement.
func (c *Client) RolloutRestart(ctx context.Context, resType, namespace, name string) error {
	// The patch sets a restart annotation on the pod template. The controller
	// sees the template changed and starts a rolling update.
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`,
		time.Now().Format(time.RFC3339),
	)
	patchBytes := []byte(patch)
	patchType := types.StrategicMergePatchType

	var err error
	switch resType {
	case "Deployments":
		_, err = c.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, patchBytes, metav1.PatchOptions{})
	case "StatefulSets":
		_, err = c.clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, patchType, patchBytes, metav1.PatchOptions{})
	case "DaemonSets":
		_, err = c.clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, patchType, patchBytes, metav1.PatchOptions{})
	default:
		return fmt.Errorf("rollout restart not supported for %s", resType)
	}
	return err
}

// GetReplicas returns the current replica count for a scalable resource.
func (c *Client) GetReplicas(ctx context.Context, resType, namespace, name string) (int, error) {
	switch resType {
	case "Deployments":
		obj, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		if obj.Spec.Replicas != nil {
			return int(*obj.Spec.Replicas), nil
		}
		return 1, nil // default is 1 if unset
	case "StatefulSets":
		obj, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		if obj.Spec.Replicas != nil {
			return int(*obj.Spec.Replicas), nil
		}
		return 1, nil
	default:
		return 0, fmt.Errorf("scale not supported for %s", resType)
	}
}

// ScaleResource sets the replica count for a Deployment or StatefulSet.
func (c *Client) ScaleResource(ctx context.Context, resType, namespace, name string, replicas int) error {
	scale, err := c.clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if resType == "StatefulSets" {
		scale, err = c.clientset.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return fmt.Errorf("getting current scale: %w", err)
	}

	replicas32 := int32(replicas)
	scale.Spec.Replicas = replicas32

	switch resType {
	case "Deployments":
		_, err = c.clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	case "StatefulSets":
		_, err = c.clientset.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	default:
		return fmt.Errorf("scale not supported for %s", resType)
	}
	return err
}
