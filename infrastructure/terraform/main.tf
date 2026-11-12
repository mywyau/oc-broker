resource "aws_sqs_queue" "claim_commands_dlq" {
  name                      = "oc-claim-commands-dlq"
  message_retention_seconds = 1209600
}

resource "aws_sqs_queue" "claim_commands" {
  name                       = "oc-claim-commands"
  receive_wait_time_seconds  = 20
  visibility_timeout_seconds = 30
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.claim_commands_dlq.arn
    maxReceiveCount     = 5
  })
}

resource "aws_sqs_queue" "claim_events" {
  name                      = "oc-claim-events"
  receive_wait_time_seconds = 20
}

output "claim_commands_queue_url" {
  value = aws_sqs_queue.claim_commands.url
}

output "claim_events_queue_url" {
  value = aws_sqs_queue.claim_events.url
}
