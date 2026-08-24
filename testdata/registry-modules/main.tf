module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "20.8.4"
}

module "unpinned" {
  source  = "terraform-aws-modules/rds/aws"
  version = ">= 6.0"
}
