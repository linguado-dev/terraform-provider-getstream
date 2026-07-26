resource "getstream_sqs" "example" {
  sqs_url        = "https://sqs.us-east-1.amazonaws.com/000000000000/example"
  sqs_access_key = "AKIAEXAMPLE"
  sqs_secret_key = "example-secret"
}
