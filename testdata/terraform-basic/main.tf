module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "6.0.1"
}

module "network" {
  source = "./modules/network"
}
