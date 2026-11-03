#!/usr/bin/env python3
"""Preserve authentication material across make local's clean regeneration."""

import json
import os
from pathlib import Path
import re
import stat
import sys
import tempfile

try:
    import yaml
except ImportError:
    sys.exit("local credential preservation requires PyYAML; install scripts/requirements-local.txt")


CREDENTIAL_FIELDS = (
    "auth.jwt.key",
    "auth.jwt.issuer",
    "auth.mfa.encryptionKeyFile",
    "auth.mfa.totpPeriod",
    "auth.challengeEncryption.privateKeyFile",
    "auth.challengeEncryption.keyId",
)
KEY_FILE_FIELDS = (
    "auth.mfa.encryptionKeyFile",
    "auth.challengeEncryption.privateKeyFile",
)


class PreservationError(Exception):
    """An error safe to display without exposing configuration or key values."""


class ConfigLoader(yaml.SafeLoader):
    def construct_mapping(self, node, deep=False):
        seen = set()
        for key, _ in node.value:
            if key.tag == "tag:yaml.org,2002:merge":
                continue
            identity = (key.tag, key.value)
            if not isinstance(key, yaml.ScalarNode) or identity in seen:
                raise PreservationError("auth.yml contains invalid or duplicate mapping keys")
            seen.add(identity)
        return super().construct_mapping(node, deep=deep)


# Match the service's YAML 1.2 boolean interpretation: an issuer/key ID such as
# 'on' or 'no' is a string, even when it was not quoted in the original file.
ConfigLoader.yaml_implicit_resolvers = {
    initial: [(tag, pattern) for tag, pattern in resolvers if tag != "tag:yaml.org,2002:bool"]
    for initial, resolvers in yaml.SafeLoader.yaml_implicit_resolvers.items()
}
ConfigLoader.add_implicit_resolver(
    "tag:yaml.org,2002:bool", re.compile(r"^(?:true|True|TRUE|false|False|FALSE)$"), list("tTfF")
)


def read_config(path):
    text = path.read_bytes().decode("utf-8")
    try:
        values = yaml.load(text, Loader=ConfigLoader)
    except yaml.YAMLError:
        raise PreservationError("auth.yml must contain a valid YAML mapping") from None
    if not isinstance(values, dict):
        raise PreservationError("auth.yml must contain a valid YAML mapping")
    return text, values


def scalar(values, field):
    value = values
    for key in field.split("."):
        if not isinstance(value, dict):
            raise PreservationError(f"invalid mapping for {field}")
        if key not in value:
            return None
        value = value[key]
    if not isinstance(value, str):
        raise PreservationError(f"{field} must be a string")
    return value


def write_file(path, data, mode):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=".credentials-", dir=path.parent)
    try:
        with os.fdopen(descriptor, "wb") as output:
            os.fchmod(output.fileno(), mode)
            output.write(data)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def capture(auth_dir, backup_dir):
    try:
        _, config = read_config(auth_dir / "conf/auth.yml")
    except FileNotFoundError:
        config = {}
    values = {}
    for field in CREDENTIAL_FIELDS:
        value = scalar(config, field)
        if value is not None:
            values[field] = value
    files, seen = [], set()
    for field in KEY_FILE_FIELDS:
        paths = [values.get(field, "")]
        override = "SERVICE_" + field.replace(".", "_").upper()
        if override in os.environ:
            paths.append(os.environ[override])
        for value in paths:
            if not value:
                continue
            path = Path(os.path.abspath(auth_dir / value))
            try:
                relative = path.relative_to(auth_dir)
            except ValueError:
                # External files stay in place. An external symlink may still
                # refer to a key inside the project that is about to be removed.
                try:
                    path = path.resolve(strict=True)
                    relative = path.relative_to(auth_dir)
                except (OSError, ValueError, RuntimeError):
                    continue
            if relative in seen:
                continue
            try:
                info = path.stat()
                if not stat.S_ISREG(info.st_mode):
                    raise OSError("not a regular file")
                data = path.read_bytes()
            except OSError:
                raise PreservationError(
                    f"{field} must reference a readable regular key file before regeneration"
                ) from None
            blob = f"credential-key-{len(files)}"
            write_file(backup_dir / blob, data, 0o600)
            files.append({"path": str(relative), "blob": blob, "mode": info.st_mode & 0o777})
            seen.add(relative)
    state = json.dumps({"values": values, "files": files}).encode("utf-8")
    write_file(backup_dir / "credentials.json", state, 0o600)


def merge_config(text, config, values):
    # Edit scalar spans in generated YAML so surrounding comments and formatting
    # survive. An unchanged rollback keeps the original file byte-for-byte.
    root = yaml.compose(text, Loader=ConfigLoader)
    edits = []
    missing = False
    for field in CREDENTIAL_FIELDS:
        if field not in values or scalar(config, field) == values[field]:
            continue
        node = root
        for key in field.split("."):
            if not isinstance(node, yaml.MappingNode):
                node = None
                break
            node = next((value for name, value in node.value if name.value == key), None)
        if not isinstance(node, yaml.ScalarNode):
            missing = True
            break
        # JSON strings are valid YAML scalars, including empty/multiline strings.
        replacement = json.dumps(values[field], ensure_ascii=True)
        # Block scalar spans include their terminating newline and indentation.
        if node.style in ("|", ">"):
            replacement += "\n" + " " * node.end_mark.column
        edits.append((node.start_mark.index, node.end_mark.index, replacement))
    if not missing:
        for start, end, replacement in sorted(edits, reverse=True):
            text = text[:start] + replacement + text[end:]
        return text
    # Older/sparse generated configurations may lack a saved path entirely.
    for field in CREDENTIAL_FIELDS:
        if field not in values:
            continue
        keys, current = field.split("."), config
        for key in keys[:-1]:
            current = current.setdefault(key, {})
            if not isinstance(current, dict):
                raise PreservationError(f"cannot restore mapping for {field}")
        current[keys[-1]] = values[field]
    return yaml.safe_dump(config, sort_keys=False, allow_unicode=True)


def restore(auth_dir, backup_dir):
    try:
        state = json.loads((backup_dir / "credentials.json").read_text(encoding="utf-8"))
        values = state["values"]
        if not isinstance(values, dict) or any(
            field not in CREDENTIAL_FIELDS or not isinstance(value, str)
            for field, value in values.items()
        ):
            raise ValueError
        files = state.get("files") or []
        for item in files:
            for field in ("path", "blob"):
                path = Path(item[field])
                if path.is_absolute() or ".." in path.parts or path == Path("."):
                    raise ValueError
            if not isinstance(item["mode"], int) or not 0 <= item["mode"] <= 0o777:
                raise ValueError
    except (ValueError, TypeError, KeyError):
        raise PreservationError("invalid credential backup manifest") from None
    for item in files:
        data = (backup_dir / item["blob"]).read_bytes()
        write_file(auth_dir / item["path"], data, item["mode"])
    if not values:
        return
    path = auth_dir / "conf/auth.yml"
    text, config = read_config(path)
    restored = merge_config(text, config, values)
    write_file(path, restored.encode("utf-8"), 0o600)


def main():
    if len(sys.argv) != 4 or sys.argv[1] not in ("snapshot", "restore"):
        print("usage: local-credentials.py snapshot|restore AUTH_DIR BACKUP_DIR", file=sys.stderr)
        return 2
    os.umask(0o077)
    auth_dir = Path(os.path.abspath(sys.argv[2]))
    backup_dir = Path(sys.argv[3])
    try:
        operation = capture if sys.argv[1] == "snapshot" else restore
        operation(auth_dir, backup_dir)
    except PreservationError as error:
        print(f"local credential preservation failed: {error}", file=sys.stderr)
        return 1
    except (OSError, UnicodeError, yaml.YAMLError, ValueError, TypeError, KeyError, AttributeError):
        # Parser/IO exceptions can contain source fragments; do not print them.
        print("local credential preservation failed: unable to read or restore credential files", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
