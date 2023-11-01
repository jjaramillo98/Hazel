package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/withmandala/go-log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	EMPTY_STRING              string = ""
	KUBE_CONFIG_RELATIVE_PATH string = "/.kube/config"
	LOG_MESSAGE_REGEX         string = `\"message\":\"(.*?)\",` // Workaround for lack of positive lookaround support in GO
	LOG_TIMESTAMP_REGEX       string = `\"env_time\":\"(.*?)\",`
)

var logger = log.New(os.Stderr).WithColor()

func PanicOnError(err *error) {
	if *err != nil {
		logger.Error("Error has occured:", *err)
		panic(*err)
	}
}

func InitializeClient() *kubernetes.Clientset {
	logger.Info("Initializing Kubernetes Client")

	homepath, _ := os.UserHomeDir()
	config, err := clientcmd.BuildConfigFromFlags(EMPTY_STRING, filepath.Join(homepath, KUBE_CONFIG_RELATIVE_PATH))

	PanicOnError(&err)

	client, err := kubernetes.NewForConfig(config)
	PanicOnError(&err)

	return client
}

func WatchAndFilterServiceLogs(client *kubernetes.Clientset, serviceName string, namespace string) {
	logger.Info("Watching logs for service:", serviceName)

	context := context.Background()

	pods, _ := client.CoreV1().Pods(namespace).List(context, metav1.ListOptions{})

	var podName string

	for _, pod := range pods.Items {
		currPod := pod.Name
		if strings.Contains(currPod, serviceName) {
			podName = currPod
			break
		}
	}

	logStream, err := client.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Follow:    true,
		Container: serviceName,
	}).Stream(context)

	PanicOnError(&err)

	messageRegex, _ := regexp.Compile(LOG_MESSAGE_REGEX)
	timeRegex, _ := regexp.Compile(LOG_TIMESTAMP_REGEX)

	for {
		// This seems like a reasonable buffer size
		buffer := make([]byte, 2048)

		bufferSize, err := logStream.Read(buffer)

		// Continue if no logs are emitted
		if bufferSize == 0 {
			continue
		}

		switch err {
		case io.EOF:
			logger.Error("Allocated buffer too small to read log")
			break
		default:
			PanicOnError(&err)
		}

		matchMessage := messageRegex.Find(buffer)
		timeMessage := timeRegex.Find(buffer)

		if matchMessage != nil && timeMessage != nil {
			message := strings.Replace(string(matchMessage), "\"message\":", "", 1)
			timestamp := strings.Replace(string(timeMessage), "\"env_time\":", "", 1)
			fmt.Printf("%s %s\n", timestamp, message)
		}

	}
}

func main() {
	var serviceName string
	var namespace string

	flag.StringVar(&serviceName, "service", "", "Name of service")
	flag.StringVar(&namespace, "namespace", "default", "Namespace of service")
	flag.Parse()

	if serviceName == "" {
		logger.Error("Service name cannot be empty")
		panic(1)
	}

	logger.Info("Creating Kubernetes Client")
	client := InitializeClient()
	logger.Info("Client Initialized.")

	WatchAndFilterServiceLogs(client, serviceName, namespace)
}
