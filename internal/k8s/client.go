package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Client wraps the Kubernetes clientset.
// In Go, it's common to create a thin wrapper like this rather than exposing
// the raw third-party type. This gives us a single place to add helper methods
// and means the rest of our app doesn't need to import client-go directly.
type Client struct {
	clientset *kubernetes.Clientset
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

	// clientcmd.BuildConfigFromFlags parses the kubeconfig file into a rest.Config.
	// The first param ("") is the master URL — empty means "read it from the config file".
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		// fmt.Errorf with %w "wraps" the original error. This preserves the chain
		// so callers can use errors.Is() or errors.As() to inspect the root cause.
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	// kubernetes.NewForConfig creates the clientset — a collection of API clients,
	// one for each Kubernetes resource type (pods, services, deployments, etc.).
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	return &Client{clientset: clientset}, nil
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

// ListPods returns the names of all pods in a given namespace.
func (c *Client) ListPods(ctx context.Context, namespace string) ([]string, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in %s: %w", namespace, err)
	}

	names := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		names = append(names, pod.Name)
	}

	return names, nil
}

// ListDeployments returns the names of all deployments in a given namespace.
// Deployments live in the "apps/v1" API group — that's why we use AppsV1()
// instead of CoreV1(). Kubernetes organises resources into API groups to keep
// things modular. Core resources (pods, services, configmaps, namespaces)
// predate the group system, so they live in CoreV1 for historical reasons.
func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]string, error) {
	depList, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments in %s: %w", namespace, err)
	}

	names := make([]string, 0, len(depList.Items))
	for _, dep := range depList.Items {
		names = append(names, dep.Name)
	}

	return names, nil
}

// ListServices returns the names of all services in a given namespace.
func (c *Client) ListServices(ctx context.Context, namespace string) ([]string, error) {
	svcList, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services in %s: %w", namespace, err)
	}

	names := make([]string, 0, len(svcList.Items))
	for _, svc := range svcList.Items {
		names = append(names, svc.Name)
	}

	return names, nil
}

// ListConfigMaps returns the names of all configmaps in a given namespace.
func (c *Client) ListConfigMaps(ctx context.Context, namespace string) ([]string, error) {
	cmList, err := c.clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing configmaps in %s: %w", namespace, err)
	}

	names := make([]string, 0, len(cmList.Items))
	for _, cm := range cmList.Items {
		names = append(names, cm.Name)
	}

	return names, nil
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
