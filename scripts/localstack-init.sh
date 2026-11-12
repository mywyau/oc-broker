#!/bin/sh
set -eu

awslocal sqs create-queue --queue-name oc-claim-commands-dlq >/dev/null
DLQ_URL=$(awslocal sqs get-queue-url --queue-name oc-claim-commands-dlq --query 'QueueUrl' --output text)
DLQ_ARN=$(awslocal sqs get-queue-attributes \
  --queue-url "${DLQ_URL}" \
  --attribute-names QueueArn --query 'Attributes.QueueArn' --output text)
awslocal sqs create-queue --queue-name oc-claim-commands \
  --attributes "{\"RedrivePolicy\":\"{\\\"deadLetterTargetArn\\\":\\\"${DLQ_ARN}\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\",\"ReceiveMessageWaitTimeSeconds\":\"20\",\"VisibilityTimeout\":\"30\"}" >/dev/null
awslocal sqs create-queue --queue-name oc-claim-events >/dev/null
