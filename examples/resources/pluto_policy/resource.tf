resource "pluto_policy" "notetakers" {
  name             = "Block AI notetakers"
  category         = "AI Notetakers"
  policy_type      = "block"
  enforcement_tool = "Falcon"
  tools            = ["Otter", "Fireflies"]

  targeted_scopes = [{
    type   = "user_department"
    values = ["Engineering"]
  }]
}
