terraform {
  source = "git::https://github.com/example/network.git?ref=v1.4.0"
}

remote_state {
  backend = "s3"
  config = {
    bucket = "tf-state"
    key    = "vpc.tfstate"
  }
}
