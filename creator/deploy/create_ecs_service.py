#!/usr/bin/env python3
import json
import os

from deploy.utils import find_and_replace

def create_ecs_service(algo_name, aws_account_id, cluster_name, region_name, subnet, security_group):

    with open('deploy/ecs_task_template.json', 'r') as f:
        ecs_task_template = json.load(f)

    with open('deploy/ecs_service_template.json', 'r') as f:
        ecs_service_template = json.load(f)

    create_task_json = find_and_replace(ecs_task_template, "ALGO_NAME", algo_name)
    create_task_json = find_and_replace(create_task_json, "AWS_REGION_NAME", region_name)
    create_task_json = find_and_replace(create_task_json, "AWS_ACCOUNT_ID", str(aws_account_id))

    # Wait till the last call to dump json to a string so that we can send it to the aws cli
    create_task_json = json.dumps(find_and_replace(create_task_json, "AWS_CLUSTER_NAME", cluster_name))
    os.system("aws ecs register-task-definition --cli-input-json '" + create_task_json + "'")

    create_service_json = find_and_replace(ecs_service_template, "ALGO_NAME", algo_name)
    create_service_json = find_and_replace(create_service_json, "AWS_SUBNET", subnet)
    create_service_json = find_and_replace(create_service_json, "AWS_ACCOUNT_ID", str(aws_account_id))

    # Wait till the last call to dump json to a string so that we can send it to the aws cli
    create_service_json = json.dumps(find_and_replace(create_service_json, "AWS_SECURITY_GROUP", security_group))
    os.system("aws ecs create-service --cluster " + cluster_name + " --launch-type FARGATE --cli-input-json '" + create_service_json + "'")