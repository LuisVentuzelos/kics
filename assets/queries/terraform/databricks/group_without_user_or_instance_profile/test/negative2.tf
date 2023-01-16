resource "databricks_instance_profile" "negative_instance_profile" {
  instance_profile_arn = "my_instance_profile_arn"
}

resource "databricks_group" "negative_group" {
  display_name = "my_group_name"
}

resource "databricks_group_instance_profile" "negative_group_instance_profile" {
  group_id            = databricks_group.negative_group.id
  instance_profile_id = databricks_instance_profile.negative_instance_profile.id
}
