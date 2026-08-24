terraform {
  source = "terraform-aws-modules/rds/aws"
}
remote_state {
  backend = "azurerm"
}
