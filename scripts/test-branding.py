#!/usr/bin/env python3
"""Check real Scaffold output and JSON embedded in the localization seed SQL."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

LAYOUT = Path(__file__).resolve().parents[1]
SCAFFOLD = os.environ.get('SCAFFOLD', 'scaffold')

with tempfile.TemporaryDirectory(prefix='auth-branding-') as temporary:
    output = Path(temporary)
    for index, (name, title, lower) in enumerate([
        ('', 'Go Cinch', 'go cinch'),
        ('acme', 'Acme', 'acme'),
        ('this is my project', 'This Is My Project', 'this is my project'),
        ('  THIS__is--MY   project  ', 'This Is My Project', 'this is my project'),
        ('true', 'True', 'true'),
    ]):
        project = output / f'auth-{index}'
        subprocess.run([
            SCAFFOLD, 'new', str(LAYOUT), '--force', f'--output-dir={output}',
            '--run-hooks=never', '--no-prompt', '--preset=full',
            f'Project={project.name}', f'brand_name={name}',
        ], check=True, capture_output=True, text=True)
        sql = (project / 'internal/infra/db/migrations/YYYYMMDDHH-04-dictionary-default-data.sql').read_text()
        values = re.findall(r"'((?:''|[^'])*)'::jsonb", sql)
        catalogs = [json.loads(value.replace("''", "'")) for value in values]
        assert len(catalogs) == 3, 'all SQL JSON values must remain parseable'
        messages = catalogs[2]['messages']
        assert messages['en-US']['frontend']['app']['journey']['description'] == f'Sign in to {title} Admin and pick up where you left off'
        assert messages['zh-CN']['frontend']['app']['journey']['description'] == f'进入 {title} Admin，继续今天的工作'
        assert f'How to use {lower}' in sql
        assert f'如何使用 {lower}' in sql
        assert (project / 'go.mod').read_text().startswith(f'module {project.name}\n')
        fixture = (project / 'internal/modules/auth/auth_test.go').read_text()
        password = lower.replace(' ', '-')[:64] + '123'
        assert f'const authTestPassword = "{password}"' in fixture
        for path in project.rglob('*'):
            relative = str(path.relative_to(project))
            assert not re.search(r'(?<!go[ _-])cinch', relative, re.I), path
            if not path.is_file():
                continue
            try:
                text = path.read_text()
            except UnicodeError:
                continue
            assert not re.search(r'(?<!go[ _-])cinch', text, re.I), path
            if name:
                assert not re.search(r'cinch', text + relative, re.I), path
        print(f'PASS auth brand {name or "default"!r}', flush=True)

    for index, invalid in enumerate(["O'Reilly", '<script>', '---', 'acme\nother']):
        project = output / f'invalid-{index}'
        result = subprocess.run([
            SCAFFOLD, 'new', str(LAYOUT), '--force', f'--output-dir={output}',
            '--run-hooks=never', '--no-prompt', f'Project={project.name}',
            f'brand_name={invalid}',
        ], capture_output=True, text=True)
        assert result.returncode != 0
        assert 'brand_name must' in result.stdout + result.stderr
        assert not project.exists(), 'invalid brand wrote project files'
    print('PASS auth invalid names rejected before rendering', flush=True)
