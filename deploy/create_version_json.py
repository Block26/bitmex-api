import subprocess
import json

from datetime import datetime
from utils import find_and_replace


commit = subprocess.run('git rev-parse --verify HEAD'.split(' '), stdout=subprocess.PIPE).stdout.decode('utf-8').replace("\n", "")
branch = subprocess.run('git rev-parse --abbrev-ref HEAD'.split(' '), stdout=subprocess.PIPE).stdout.decode('utf-8').replace("\n", "")
lasttag = subprocess.run('git describe --abbrev=0 --tags --always'.split(' '), stdout=subprocess.PIPE).stdout.decode('utf-8').replace("\n", "")

with open('deploy/template_version.json', 'r') as f:
    template = json.load(f)

templated = find_and_replace(template, '{{branch}}', branch)
templated = find_and_replace(templated, '{{commit}}', commit)
templated = find_and_replace(templated, '{{lasttag}}', lasttag)
templated = find_and_replace(templated, '{{buildtime}}', str(datetime.now()))

with open('version.json', 'w', encoding='utf-8') as f:
    json.dump(templated, f, ensure_ascii=False, indent=4)