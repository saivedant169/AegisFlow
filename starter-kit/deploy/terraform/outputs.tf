output "ecs_cluster_name" {
  description = "ECS cluster name"
  value       = aws_ecs_cluster.aegisflow.name
}

output "ecs_service_name" {
  description = "ECS service name"
  value       = aws_ecs_service.aegisflow.name
}

output "ecr_repository_url" {
  description = "ECR repository URL for pushing AegisFlow images"
  value       = aws_ecr_repository.aegisflow.repository_url
}

output "security_group_id" {
  description = "Security group ID for the AegisFlow service"
  value       = aws_security_group.aegisflow.id
}

output "task_definition_arn" {
  description = "ARN of the ECS task definition"
  value       = aws_ecs_task_definition.aegisflow.arn
}

output "log_group_name" {
  description = "CloudWatch log group name"
  value       = aws_cloudwatch_log_group.aegisflow.name
}
