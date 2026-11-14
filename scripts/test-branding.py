#!/usr/bin/env python3
"""Check rendered branding, domains, email defaults and embedded seed JSON."""
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
    for index, (name, title, lower, domain_input, domain) in enumerate([
        ('', 'Go Cinch', 'go cinch', None, 'go-cinch.top'),
        ('acme', 'Acme', 'acme', '', 'go-cinch.top'),
        ('acme', 'Acme', 'acme', '   ', 'go-cinch.top'),
        ('this is my project', 'This Is My Project', 'this is my project', 'brand.example.co.uk', 'brand.example.co.uk'),
        ('  THIS__is--MY   project  ', 'This Is My Project', 'this is my project', '  MAIL.Brand-2.EXAMPLE  ', 'mail.brand-2.example'),
        ('true', 'True', 'true', 'xn--bcher-kva.example', 'xn--bcher-kva.example'),
        ('zenwell', 'Zenwell', 'zenwell', 'zenwell.example', 'zenwell.example'),
    ]):
        project = output / f'auth-{index}'
        domain_args = [] if domain_input is None else [f'brand_domain={domain_input}']
        preset_args = [] if index == 0 else ['--preset=full']
        subprocess.run([
            SCAFFOLD, 'new', str(LAYOUT), '--force', f'--output-dir={output}',
            '--run-hooks=never', '--no-prompt', *preset_args,
            f'Project={project.name}', f'brand_name={name}',
            *domain_args,
        ], check=True, capture_output=True, text=True)
        auth_config = (project / 'conf/auth.yml').read_text()
        assert f'accountNamePrefix: "{lower} admin"' in auth_config
        http_config = (project / 'conf/http.yml').read_text()
        initial_docs = (project / 'internal/docs/openapi.yaml').read_text()
        for text in [http_config, initial_docs]:
            assert f'url: "https://{domain}/api/{project.name}"' in text
            assert f'url: "https://dev.{domain}/api/{project.name}"' in text
            assert 'url: "http://127.0.0.1:8080"' in text
        email_config = (project / 'conf/email.yml').read_text()
        for key, value in {
            'host': 'smtp.resend.com', 'username': 'resend', 'password': '',
            'from': f'no-reply@{domain}', 'fromName': title,
        }.items():
            assert f'  {key}: "{value}"\n' in email_config, key
        sql = (project / 'internal/infra/db/migrations/YYYYMMDDHH-04-dictionary-default-data.sql').read_text()
        values = re.findall(r"'((?:''|[^'])*)'::jsonb", sql)
        catalogs = [json.loads(value.replace("''", "'")) for value in values]
        assert len(catalogs) == 3, 'all SQL JSON values must remain parseable'
        messages = catalogs[2]['messages']
        assert messages['en-US']['frontend']['app']['journey']['description'] == f'Sign in to {title} Admin and pick up where you left off'
        assert messages['zh-CN']['frontend']['app']['journey']['description'] == f'进入 {title} Admin，继续今天的工作'
        assert f'How to use {lower}' in sql
        assert f'如何使用 {lower}' in sql
        preferences = json.loads(re.search(r'\$preferences\$(.*?)\$preferences\$', sql, re.S)[1])
        assert preferences['copyright']['company_site_link'] == f'https://{domain}'
        mail_sql = (project / 'internal/infra/db/migrations/YYYYMMDDHH-05-mail-template-default-data.sql').read_text()
        mail_templates = re.findall(r'\$template\$(.*?)\$template\$', mail_sql, re.S)
        assert mail_templates, 'mail template seed JSON must be checked'
        for value in mail_templates:
            template = json.loads(value)
            for locale in template.values():
                assert locale['text'].startswith(title + '\n')
                assert f'alt="{title}"' in locale['html']
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
                # The domain is independent and may intentionally keep its default.
                assert not re.search(r'cinch', text.replace(domain, '') + relative, re.I), path
        print(f'PASS auth brand {name or "default"!r}, domain {domain!r}', flush=True)

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

    for index, invalid in enumerate([
        'https://brand.example', 'brand.example/path', 'brand.example:443',
        'no-reply@brand.example', '*.brand.example', 'brand..example',
        '-brand.example', 'brand-.example', 'brand_name.example', 'localhost',
        '127.0.0.1', 'brand.example.', 'brand.example\nother', "brand'.example",
        'a' * 64 + '.example', '.'.join(['a' * 63] * 4),
    ]):
        project = output / f'invalid-domain-{index}'
        result = subprocess.run([
            SCAFFOLD, 'new', str(LAYOUT), '--force', f'--output-dir={output}',
            '--run-hooks=never', '--no-prompt', f'Project={project.name}',
            f'brand_domain={invalid}',
        ], capture_output=True, text=True)
        assert result.returncode != 0, invalid
        assert 'brand_domain must' in result.stdout + result.stderr, invalid
        assert not project.exists(), 'invalid domain wrote project files'
    print('PASS auth invalid domains rejected before rendering', flush=True)
