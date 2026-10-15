# A one-off task definition, never a service or an automatic Terraform provisioner.
# Operators need separately controlled ecs:RunTask/iam:PassRole permission.
locals {
  ops_enabled = var.migration_database_secret_arn != "" && var.runtime_database_password_secret_arn != ""
}
resource "aws_cloudwatch_log_group" "operations" {
  count             = local.ops_enabled ? 1 : 0
  name              = "/ecs/${local.name}/database-operations"
  retention_in_days = 90
}
resource "aws_iam_role" "ops_execution" {
  count              = local.ops_enabled ? 1 : 0
  name               = "${local.name}-database-ops-execution"
  assume_role_policy = aws_iam_role.execution.assume_role_policy
}
resource "aws_iam_role_policy_attachment" "ops_execution" {
  count      = local.ops_enabled ? 1 : 0
  role       = aws_iam_role.ops_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
resource "aws_iam_role_policy" "ops_secrets" {
  count = local.ops_enabled ? 1 : 0
  role  = aws_iam_role.ops_execution[0].id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect   = "Allow", Action = ["secretsmanager:GetSecretValue"],
    Resource = [var.migration_database_secret_arn, var.runtime_database_password_secret_arn, var.secret_arns["DATABASE_URL"]]
  }] })
}
resource "aws_iam_role" "ops_task" {
  count              = local.ops_enabled ? 1 : 0
  name               = "${local.name}-database-ops"
  assume_role_policy = aws_iam_role.execution.assume_role_policy
}
resource "aws_ecs_task_definition" "database_operations" {
  count                    = local.ops_enabled ? 1 : 0
  family                   = "${local.name}-database-operations"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.ops_execution[0].arn
  task_role_arn            = aws_iam_role.ops_task[0].arn
  container_definitions = jsonencode([{
    name       = "database-operations", essential = true,
    image      = "${aws_ecr_repository.services["api"].repository_url}:${var.image_tag}",
    entryPoint = ["/usr/local/bin/cloud-migrate"], command = ["check"],
    secrets = [
      { name = "MIGRATION_DATABASE_URL", valueFrom = var.migration_database_secret_arn },
      { name = "DATABASE_URL", valueFrom = var.secret_arns["DATABASE_URL"] },
      { name = "RUNTIME_DATABASE_PASSWORD", valueFrom = var.runtime_database_password_secret_arn }
    ],
    logConfiguration = { logDriver = "awslogs", options = {
      "awslogs-group"  = aws_cloudwatch_log_group.operations[0].name,
      "awslogs-region" = var.region, "awslogs-stream-prefix" = "database-operations"
    } }
  }])
}
output "database_operations_task_definition" {
  value = local.ops_enabled ? aws_ecs_task_definition.database_operations[0].arn : null
}
output "private_subnet_ids" { value = aws_subnet.private[*].id }
output "task_security_group_id" { value = aws_security_group.tasks.id }
output "ecs_cluster_name" { value = aws_ecs_cluster.main.name }
