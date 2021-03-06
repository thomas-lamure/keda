package scalers

import (
	"context"

	v2beta1 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type Scaler interface {

	// The scaler returns the metric values for a metric Name and criteria matching the selector
	GetMetrics(ctx context.Context, metricName string, metricSelector labels.Selector) ([]external_metrics.ExternalMetricValue, error)

	//returns the metrics based on which this scaler determines that the deployment scales. This is used to contruct the HPA spec that is created for
	// this scaled object. The labels used should match the selectors used in GetMetrics
	GetMetricSpecForScaling() []v2beta1.MetricSpec

	//returns the metrics based on which this scaler determines that the job scales. The labels used should match the selectors used in GetMetrics
	GetMetricSpecForScalingJob() []v2beta1.MetricSpec

	IsActive(ctx context.Context) (bool, error)

	// Close any resources that need disposing when scaler is no longer used or destroyed
	Close() error
}
