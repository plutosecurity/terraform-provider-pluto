terraform {
  required_providers {
    pluto = {
      source = "plutosecurity/pluto"
    }
  }
}

provider "pluto" {
  host = "https://api.example.pluto.security" # or PLUTO_HOST
  # api_key is read from PLUTO_API_KEY: a pluto_... key with policy write access
}
