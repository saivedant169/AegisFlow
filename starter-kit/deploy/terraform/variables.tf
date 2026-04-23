variable "aws_region" {
  description = "AWS region to deploy into"
  type        = string
  default     = "us-east-1"
}

variable "vpc_id" {
  description = "VPC ID to deploy AegisFlow into"
  type        = string
}

variable "name_prefix" {
  description = "Prefix for all resource names"
  type        = string
  default     = "aegisflow"
}

variable "image_tag" {
  description = "Docker image tag to deploy"
  type        = string
  default     = "0.7.0"
}

variable "task_cpu" {
  description = "Fargate task CPU units (1 vCPU = 1024)"
  type        = string
  default     = "256"
}

variable "task_memory" {
  description = "Fargate task memory in MB"
  type        = string
  default     = "512"
}

variable "desired_count" {
  description = "Number of ECS tasks to run"
  type        = number
  default     = 1
}
