module "network" {
  source = "git::https://github.com/example/network.git?ref=v1.4.0"
}

module "sshmod" {
  source = "git::ssh://git@github.com/example/vpn.git?ref=abc123"
}
