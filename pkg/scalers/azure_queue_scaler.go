package scalers

import (
	"context"
	"fmt"
	"strconv"

	v2beta1 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	queueLengthMetricName        = "queueLength"
	visibleQueueLengthMetricName = "visibleQueueLength"
	defaultTargetQueueLength     = 5
	externalMetricType           = "External"
	defaultConnectionSetting     = "AzureWebJobsStorage"
)

type azureQueueScaler struct {
	metadata    *azureQueueMetadata
	podIdentity string
}

type azureQueueMetadata struct {
	targetQueueLength int
	queueName         string
	connection        string
	useAAdPodIdentity bool
	accountName       string
}

var azureQueueLog = logf.Log.WithName("azure_queue_scaler")

func NewAzureQueueScaler(resolvedEnv, metadata, authParams map[string]string, podIdentity string) (Scaler, error) {
	meta, podIdentity, err := parseAzureQueueMetadata(metadata, resolvedEnv, authParams, podIdentity)
	if err != nil {
		return nil, fmt.Errorf("error parsing azure queue metadata: %s", err)
	}

	return &azureQueueScaler{
		metadata:    meta,
		podIdentity: podIdentity,
	}, nil
}

func parseAzureQueueMetadata(metadata, resolvedEnv, authParams map[string]string, podAuth string) (*azureQueueMetadata, string, error) {
	meta := azureQueueMetadata{}
	meta.targetQueueLength = defaultTargetQueueLength

	if val, ok := metadata[queueLengthMetricName]; ok {
		queueLength, err := strconv.Atoi(val)
		if err != nil {
			azureQueueLog.Error(err, "Error parsing azure queue metadata", "queueLengthMetricName", queueLengthMetricName)
			return nil, "", fmt.Errorf("Error parsing azure queue metadata %s: %s", queueLengthMetricName, err.Error())
		}

		meta.targetQueueLength = queueLength
	}

	if val, ok := metadata["queueName"]; ok && val != "" {
		meta.queueName = val
	} else {
		return nil, "", fmt.Errorf("no queueName given")
	}

	// before triggerAuthentication CRD, pod identity was configured using this property
	if val, ok := metadata["useAAdPodIdentity"]; ok && podAuth == "" {
		if val == "true" {
			podAuth = "azure"
		}
	}

	// If the Use AAD Pod Identity is not present, or set to "none"
	// then check for connection string
	if podAuth == "" || podAuth == "none" {
		// Azure Queue Scaler expects a "connection" parameter in the metadata
		// of the scaler or in a TriggerAuthentication object
		connection := authParams["connection"]
		if connection != "" {
			// Found the connection in a parameter from TriggerAuthentication
			meta.connection = connection
		} else {
			connectionSetting := defaultConnectionSetting
			if val, ok := metadata["connection"]; ok && val != "" {
				connectionSetting = val
			}

			if val, ok := resolvedEnv[connectionSetting]; ok {
				meta.connection = val
			} else {
				return nil, "", fmt.Errorf("no connection setting given")
			}
		}
	} else if podAuth == "azure" {
		// If the Use AAD Pod Identity is present then check account name
		if val, ok := metadata["accountName"]; ok && val != "" {
			meta.accountName = val
		} else {
			return nil, "", fmt.Errorf("no accountName given")
		}
	} else {
		return nil, "", fmt.Errorf("pod identity %s not supported for azure storage queues", podAuth)
	}

	return &meta, podAuth, nil
}

// GetScaleDecision is a func
func (s *azureQueueScaler) IsActive(ctx context.Context) (bool, error) {
	length, err := GetAzureQueueLength(
		ctx,
		s.podIdentity,
		s.metadata.connection,
		s.metadata.queueName,
		s.metadata.accountName,
	)

	if err != nil {
		azureQueueLog.Error(err, "error)")
		return false, err
	}

	return length > 0, nil
}

func (s *azureQueueScaler) Close() error {
	return nil
}

func (s *azureQueueScaler) GetMetricSpecForScaling() []v2beta1.MetricSpec {
	targetQueueLengthQty := resource.NewQuantity(int64(s.metadata.targetQueueLength), resource.DecimalSI)
	externalMetric := &v2beta1.ExternalMetricSource{MetricName: queueLengthMetricName, TargetAverageValue: targetQueueLengthQty}
	metricSpec := v2beta1.MetricSpec{External: externalMetric, Type: externalMetricType}
	return []v2beta1.MetricSpec{metricSpec}
}

func (s *azureQueueScaler) GetMetricSpecForScalingJob() []v2beta1.MetricSpec {
	targetQueueLengthQty := resource.NewQuantity(int64(s.metadata.targetQueueLength), resource.DecimalSI)
	externalMetric := &v2beta1.ExternalMetricSource{MetricName: visibleQueueLengthMetricName, TargetAverageValue: targetQueueLengthQty}
	metricSpec := v2beta1.MetricSpec{External: externalMetric, Type: externalMetricType}
	return []v2beta1.MetricSpec{metricSpec}
}

//GetMetrics returns value for a supported metric and an error if there is a problem getting the metric
func (s *azureQueueScaler) GetMetrics(ctx context.Context, metricName string, metricSelector labels.Selector) ([]external_metrics.ExternalMetricValue, error) {
	var err error
	var queuelen int32
	err = nil
	queuelen = 0
	switch metricName {
	case queueLengthMetricName:
		queuelen, err = GetAzureQueueLength(
			ctx,
			s.podIdentity,
			s.metadata.connection,
			s.metadata.queueName,
			s.metadata.accountName,
		)
	case visibleQueueLengthMetricName:
		queuelen, err = GetAzureVisibleQueueLength(
			ctx,
			s.podIdentity,
			s.metadata.connection,
			s.metadata.queueName,
			s.metadata.accountName,
			int32(s.metadata.targetQueueLength),
		)
	default:
		return []external_metrics.ExternalMetricValue{}, fmt.Errorf("no metric found with name: %s", metricName)
	}

	if err != nil {
		azureQueueLog.Error(err, "error getting queue length")
		return []external_metrics.ExternalMetricValue{}, err
	}

	metric := external_metrics.ExternalMetricValue{
		MetricName: metricName,
		Value:      *resource.NewQuantity(int64(queuelen), resource.DecimalSI),
		Timestamp:  metav1.Now(),
	}

	return append([]external_metrics.ExternalMetricValue{}, metric), nil
}
