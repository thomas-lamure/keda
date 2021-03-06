package scalers

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/azure-storage-queue-go/azqueue"
)

// GetAzureQueueURL returns a ready endpoint to comunicate with the API
func GetAzureQueueURL(ctx context.Context, podIdentity string, connectionString, queueName string, accountName string) (azqueue.QueueURL, error) {

	var credential azqueue.Credential
	var err error

	if podIdentity == "" || podIdentity == "none" {

		var accountKey string

		_, accountName, accountKey, _, err = ParseAzureStorageConnectionString(connectionString)

		if err != nil {
			return azqueue.QueueURL{}, err
		}

		credential, err = azqueue.NewSharedKeyCredential(accountName, accountKey)
		if err != nil {
			return azqueue.QueueURL{}, err
		}
	} else if podIdentity == "azure" {
		token, err := getAzureADPodIdentityToken("https://storage.azure.com/")
		if err != nil {
			azureQueueLog.Error(err, "Error fetching token cannot determine queue size")
			return azqueue.QueueURL{}, err
		}

		credential = azqueue.NewTokenCredential(token.AccessToken, nil)
	} else {
		return azqueue.QueueURL{}, fmt.Errorf("Azure queues doesn't support %s pod identity type", podIdentity)

	}

	p := azqueue.NewPipeline(credential, azqueue.PipelineOptions{})
	u, _ := url.Parse(fmt.Sprintf("https://%s.queue.core.windows.net", accountName))
	serviceURL := azqueue.NewServiceURL(*u, p)
	queueURL := serviceURL.NewQueueURL(queueName)
	_, err = queueURL.Create(ctx, azqueue.Metadata{})
	if err != nil {
		return azqueue.QueueURL{}, err
	}

	return queueURL, nil
}

// GetAzureQueueLength returns the length of a queue in int
func GetAzureQueueLength(ctx context.Context, podIdentity string, connectionString, queueName string, accountName string) (int32, error) {

	var err error

	queueURL, err := GetAzureQueueURL(ctx, podIdentity, connectionString, queueName, accountName)
	if err != nil {
		return -1, err
	}

	props, err := queueURL.GetProperties(ctx)
	if err != nil {
		return -1, err
	}

	return props.ApproximateMessagesCount(), nil
}

// GetAzureVisibleQueueLength returns the number of visible messages in a queue in int
func GetAzureVisibleQueueLength(ctx context.Context, podIdentity string, connectionString, queueName string, accountName string, maxCount int32) (int32, error) {

	var err error

	queueURL, err := GetAzureQueueURL(ctx, podIdentity, connectionString, queueName, accountName)
	if err != nil {
		return -1, err
	}

	pmr, err := queueURL.NewMessagesURL().Peek(ctx, maxCount)
	if err != nil {
		return -1, err
	}
	count := pmr.NumMessages()

	return count, nil
}
