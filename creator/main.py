# -*- coding: utf-8 -*-
from __future__ import print_function, unicode_literals
import os, shutil
import fileinput
import regex
import json

from pprint import pprint
from PyInquirer import style_from_dict, Token, prompt, Separator
from PyInquirer import Validator, ValidationError

from deploy.create_ecs_service import create_ecs_service

DEFAULTS = {}

with open('defaults.json', 'r') as f:
    DEFAULTS = json.load(f)

class NameValidator(Validator):
    def validate(self, document):
        ok = regex.match('^[a-z-]+$', document.text)
        if not ok:
            raise ValidationError(
                message='Names must be lower case, only letters and - allowed.',
                cursor_position=len(document.text))  # Move cursor to end

class DeploymentNameValidator(Validator):
    def validate(self, document):
        ok = regex.match('^[0-9\a-z-]+$', document.text)
        if not ok:
            raise ValidationError(
                message='Names must be lower case, only letters, numbers, and - allowed.',
                cursor_position=len(document.text))  # Move cursor to end

questions = [
    {
        'type': 'list',
        'name': 'action',
        'message': 'What do you want to do?',
        'choices': [
            'create',
            'deploy',
        ]
    },
    # CREATE PATH
    {
        'type': 'list',
        'name': 'action_type',
        'message': 'What do you want to create?',
        'when': lambda answers: answers['action'] == 'create',
        'choices': [
            'module',
            'algo',
        ]
    },
    {
        'type': 'input',
        'name': 'module_name',
        'when': lambda answers: answers['action'] == 'create' and answers['action_type'] == 'module',
        'message': 'What would you like to name the module?',
        'validate': NameValidator
    },
    {
        'type': 'input',
        'name': 'module_name',
        'when': lambda answers: answers['action'] == 'create' and answers['action_type'] == 'algo',
        'message': 'What is the name of a module this algo will use?',
        'validate': NameValidator
    },
    {
        'type': 'input',
        'name': 'algo_name',
        'when': lambda answers: (answers['action'] == 'create' and answers['action_type'] == 'algo' ),
        'message': 'What would you like to name the algo?',
        'validate': NameValidator
    },
    {
        'type': 'input',
        'name': "INSTALL_LOCATION",
        'when': lambda answers: answers['action'] == 'create',
        'default': DEFAULTS["INSTALL_LOCATION"] if "INSTALL_LOCATION" in DEFAULTS else os.environ.get('GOPATH', os.getcwd()),
        'message': 'Where should the files be placed?',
    },
    # DEPLOY PATH
    {
        'type': 'input',
        'name': 'DEPLOYMENT_NAME',
        'when': lambda answers: answers['action'] == 'deploy',
        'message': 'What is the name of this deployment?',
        'validate': DeploymentNameValidator
    },
    {
        'type': 'input',
        'name': 'EXCHANGE_NAME',
        'when': lambda answers: answers['action'] == 'deploy',
        'message': 'What exchange?',
        'validate': NameValidator
    },
    {
        'type': 'input',
        'name': 'SYMBOL_NAME',
        'when': lambda answers: answers['action'] == 'deploy',
        'message': 'What symbol?',
    },
    {
        'type': 'input',
        'name': 'SECRET_FILE',
        'when': lambda answers: answers['action'] == 'deploy',
        'message': 'What AWS is your secret name?',
    },
    {
        'type': 'input',
        'name': 'GITHUB_URL',
        'when': lambda answers: answers['action'] == 'deploy',
        'message': 'What is git url for this algo?'
    },
    {
        'type': 'input',
        'name': 'COMMIT_HASH',
        'when': lambda answers: answers['action'] == 'deploy',
        'message': 'What commit hash you are deploying?'
    },
    {
        'type': 'input',
        'name': 'BRANCH_NAME',
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS['BRANCH_NAME'],
        'message': 'What branch of the algo are you deploying?'
    },
    {
        'type': 'input',
        'name': "INSTALL_LOCATION",
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS["INSTALL_LOCATION"] if "INSTALL_LOCATION" in DEFAULTS else os.environ.get('GOPATH', os.getcwd()),
        'message': 'Where should the deploy files be placed?',
    },
    {
        'type': 'input',
        'name': 'AWS_ACCOUNT_ID',
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS["AWS_ACCOUNT_ID"],
        'message': 'What AWS Account ID deploying to?',
    },
    {
        'type': 'input',
        'name': 'AWS_CLUSTER_NAME',
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS["AWS_CLUSTER_NAME"],
        'message': 'What ECS cluster are you deploying to?',
    },
    {
        'type': 'input',
        'name': 'AWS_REGION_NAME',
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS["AWS_REGION_NAME"] ,
        'message': 'What region are you deploying to?',
    },
    {
        'type': 'input',
        'name': 'AWS_SUBNET',
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS["AWS_SUBNET"] ,
        'message': 'What subnet are you deploying to?',
    },
    {
        'type': 'input',
        'name': 'AWS_SECURITY_GROUP',
        'when': lambda answers: answers['action'] == 'deploy',
        'default': DEFAULTS["AWS_SECURITY_GROUP"] ,
        'message': 'What security group are you deploying to?',
    },
]

answers = prompt(questions)
def copy(src, dst):
    try:
        shutil.copytree(src, dst)
    except OSError as exc: # python >2.5
        if exc.errno == exc.ENOTDIR:
            shutil.copy(src, dst)
        else: raise

def replace(install_location, file_name, find, replace):
    with fileinput.FileInput(install_location + '/' + file_name, inplace=True) as file:
        for line in file:
            print(line.replace(find, replace), end='')

if answers['action'] == 'create':
    if answers['action_type'] == 'module':
        install_location = answers["INSTALL_LOCATION"] + '/' + answers['module_name']
        copy('create/module_template', install_location)
        replace(install_location, 'go.mod', 'TEMPLATE', answers['module_name'])
        replace(install_location, 'module_test.go', 'TEMPLATE', answers['module_name'])
        replace(install_location, 'module.go', 'TEMPLATE', answers['module_name'])
        replace(install_location, 'params.go', 'TEMPLATE', answers['module_name'])

    elif answers['action_type'] == 'algo':
        install_location = answers["INSTALL_LOCATION"] + '/' + answers['algo_name']
        copy('create/algo_template', install_location)

        replace(install_location, '.gitignore', 'REPLACEMEWITHSETTINGS', "settings")

        replace(install_location, 'main.go', 'TEMPLATE-STRATEGY', answers['module_name'])
        replace(install_location, 'main_test.go', 'TEMPLATE-STRATEGY', answers['module_name'])
        replace(install_location, 'optimize.go', 'TEMPLATE-STRATEGY', answers['module_name'])

        replace(install_location, 'go.mod', 'TEMPLATE', answers['algo_name'])
        replace(install_location, 'main.go', 'TEMPLATE', answers['algo_name'])
        replace(install_location, 'optimize.go', 'TEMPLATE', answers['algo_name'])

        replace(install_location, '.circleci/config.yml', 'TEMPLATE', answers['algo_name'])

    print("""
====================================================================================
__   __ _    _   _ _____ ____      _       ____ ____  _____    _  _____ _____ ____ 
\ \ / // \  | \ | |_   _|  _ \    / \     / ___|  _ \| ____|  / \|_   _| ____|  _  \ 
 \ V // _ \ |  \| | | | | |_) |  / _ \   | |   | |_) |  _|   / _ \ | | |  _| | | | |
  | |/ ___ \| |\  | | | |  _ <  / ___ \  | |___|  _ <| |___ / ___ \| | | |___| |_| |
  |_/_/   \_\_| \_| |_| |_| \_\/_/   \_\  \____|_| \_\_____/_/   \_\_| |_____|____/

====================================================================================
""")

elif answers['action'] == 'deploy':
    install_location = answers["INSTALL_LOCATION"] + '/deploy-' + answers['DEPLOYMENT_NAME']
    copy('deploy/deploy_template', install_location)

    # SETUP CONFIG
    replace(install_location, 'config.json', 'GITHUB_URL', answers['GITHUB_URL'])
    replace(install_location, 'config.json', 'DEPLOYMENT_NAME', answers['DEPLOYMENT_NAME'])
    replace(install_location, 'config.json', 'COMMIT_HASH', answers['COMMIT_HASH'])
    replace(install_location, 'config.json', 'EXCHANGE_NAME', answers['EXCHANGE_NAME'])
    replace(install_location, 'config.json', 'SYMBOL_NAME', answers['SYMBOL_NAME'])
    replace(install_location, 'config.json', 'SECRET_FILE', answers['SECRET_FILE'])
    replace(install_location, 'config.json', 'BRANCH_NAME', answers['BRANCH_NAME'])

    replace(install_location, 'go.mod', 'ALGO', answers['DEPLOYMENT_NAME'])
    replace(install_location, '.circleci/config.yml', 'ALGO', answers['DEPLOYMENT_NAME'])
    replace(install_location, '.circleci/config.yml', 'AWS_CLUSTER_NAME', answers['AWS_CLUSTER_NAME'])

    create_ecs_service(answers['DEPLOYMENT_NAME'], answers['AWS_ACCOUNT_ID'], answers['AWS_CLUSTER_NAME'], answers['AWS_REGION_NAME'], answers['AWS_SUBNET'], answers['AWS_SECURITY_GROUP'])

    DEFAULTS['DEPLOYMENT_NAME'] = answers['DEPLOYMENT_NAME']
    DEFAULTS['AWS_ACCOUNT_ID'] = answers['AWS_ACCOUNT_ID']
    DEFAULTS['AWS_CLUSTER_NAME'] = answers['AWS_CLUSTER_NAME']
    DEFAULTS['AWS_REGION_NAME'] = answers['AWS_REGION_NAME']
    DEFAULTS['AWS_SUBNET'] = answers['AWS_SUBNET']
    DEFAULTS['AWS_SECURITY_GROUP'] = answers['AWS_SECURITY_GROUP']
    print("========================================================================================")
    print("An amazon task has been created")
    print("There are a few more steps to complete the deployment")
    print("1) Create an aws secret and put it's path in the base.Connect function.")
    print("2) Configure CircleCI to build this repo. Add user key for private checkouts and ensure successful build. \n TODO USE GITHUB ACTIONS INSTEAD")

    print("""
========================================================================================
__   __ _    _   _ _____ ____      _      ____  _____ ____  _     _____   _______ ____
\ \ / // \  | \ | |_   _|  _ \    / \    |  _ \| ____|  _ \| |   / _ \ \ / / ____|  _ \
 \ V // _ \ |  \| | | | | |_) |  / _ \   | | | |  _| | |_) | |  | | | \ V /|  _| | | | |
  | |/ ___ \| |\  | | | |  _ <  / ___ \  | |_| | |___|  __/| |__| |_| || | | |___| |_| |
  |_/_/   \_\_| \_| |_| |_| \_\/_/   \_\ |____/|_____|_|   |_____\___/ |_| |_____|____/

========================================================================================
""")

DEFAULTS['INSTALL_LOCATION'] = answers["INSTALL_LOCATION"]

os.remove("defaults.json")
with open('defaults.json', 'w') as fp:
    json.dump(DEFAULTS, fp)


