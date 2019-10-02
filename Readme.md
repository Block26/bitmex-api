# Deployment
Open `deploy/create_ecs_service.py` and change `algo_name` to be equal to the new algos name. Then run the script to create an AWS ECS task and service to allow for CI/CD.
```sh
python create_ecs_service.py
```