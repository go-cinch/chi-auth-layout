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
        assert f'Sign in to {title} and pick up where you left off' in sql
        assert f'进入 {title}，继续今天的工作' in sql
        assert f'How to use {lower}' in sql
        assert f'如何使用 {lower}' in sql
        assert (project / 'go.mod').read_text().startswith(f'module {project.name}\n')
        if name:
            for path in project.rglob('*'):
                if path.is_file():
                    try:
                        text = path.read_text()
                    except UnicodeError:
                        continue
                    assert not re.search(r'go[ _-]*cinch', text, re.I), path
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
