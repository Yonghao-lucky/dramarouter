#!/usr/bin/env python3
"""Bind a manual AI handoff in one PR to its approved plan and exact source tree."""

import argparse
import copy
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tempfile

import upstream_sync as engine


def require(condition, message):
    if not condition:
        raise engine.SyncError(message)


def blob(repo, revision, path, optional=False, mode=None):
    result = engine.git(repo, "show", f"{revision}:{path}", check=False)
    if optional and result.returncode:
        return None
    require(result.returncode == 0, f"Missing committed file {revision}:{path}.")
    entry = engine.git_text(repo, "ls-tree", revision, "--", path).split("\t", 1)[0].split()
    require(len(entry) == 3 and entry[0] in {"100644", "100755"} and entry[1] == "blob",
            "Handoff records and patches must be regular Git files.")
    require(mode is None or entry[0] == mode, "Generated metadata must retain its exact regular-file mode.")
    return result.stdout


def json_blob(repo, revision, path):
    return json.loads(blob(repo, revision, path))


def normalized_path(value):
    require(isinstance(value, str) and value and "\\" not in value
            and not any(ord(character) < 32 for character in value), "Invalid repository path.")
    require(not PurePosixPath(value).is_absolute()
            and all(part not in {"", ".", ".."} for part in value.split("/")),
            "Repository paths must be exact normalized relative file paths.")
    return value


def changed_paths(repo, before, after):
    return sorted(path.decode("utf-8") for path in engine.git(
        repo, "diff", "--no-renames", "--name-only", "-z", before, after).stdout.split(b"\x00") if path)


def context_paths(plan, plan_path):
    directory = f"docs/upstream-sync/plans/{plan['batch_id']}"
    require(plan_path == directory + "/plan.json", "Plan must use its canonical committed batch path.")
    paths = {directory + "/" + name for name in ("plan.json", "report.md", "AI_HANDOFF.md")}
    for item in plan["items"]:
        patch = item.get("patch")
        if patch is not None:
            path = normalized_path(patch["path"])
            require(path.startswith("docs/upstream-sync/patches/") and path.endswith(".patch"),
                    "Reviewed patches must use the upstream-sync patches directory.")
            paths.add(path)
    return paths


def validate_plan_pins(repo, plan, revision, planning=False):
    """Validate against the frozen main tree, without candidate metadata influencing it."""
    with tempfile.TemporaryDirectory(prefix="upstream-handoff-plan-") as directory:
        root = Path(directory)
        checkout = root / "base"
        engine.git(repo, "worktree", "add", "--detach", str(checkout), plan["downstream"]["base_sha"])
        try:
            reviewed = copy.deepcopy(plan)
            if planning:
                for item in reviewed["items"]:
                    item.update({"decision": "defer", "reason": "Planning only; source changes are not authorized.",
                                 "selected_scope": "full_commit", "patch": None, "evidence": [], "dependencies": []})
                    item.pop("ai_scope", None)
            patch_root = root / "input"
            patch_root.mkdir()
            for item in reviewed["items"]:
                if item.get("patch"):
                    path = normalized_path(item["patch"]["path"])
                    destination = engine.repository_path(patch_root, path)
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_bytes(blob(repo, revision, path))
            plan_file = root / "plan.json"
            engine.write_json(plan_file, reviewed)
            engine.validate_plan(checkout, plan_file, patch_root)
        finally:
            engine.git(repo, "worktree", "remove", "--force", str(checkout))


def validate_approval(repo, plan, plan_path, approval):
    require(isinstance(approval, str) and engine.SHA_RE.fullmatch(approval), "Approval must be a full commit SHA.")
    base = plan["downstream"]["base_sha"]
    require(engine.is_ancestor(repo, base, approval), "Approval must descend from the pinned main base.")
    require(json_blob(repo, approval, plan_path) == plan, "The current plan differs from the human approval commit.")
    allowed = context_paths(plan, plan_path)
    paths = changed_paths(repo, base, approval)
    require(set(paths) <= allowed, "Source changes cannot be hidden in planning commits before human approval.")
    for path in paths:
        require(blob(repo, base, path, optional=True) is None,
                "A new batch cannot replace existing planning files or reviewed patches.")
        blob(repo, approval, path)
    validate_plan_pins(repo, plan, approval)
    selected = []
    scope = set()
    for item in plan["items"]:
        if item["decision"] not in engine.ADOPTED:
            continue
        approved_scope = item.get("ai_scope")
        require(isinstance(approved_scope, dict), "Every manually implemented selected item needs an approved ai_scope.")
        for path in approved_scope["paths"]:
            normalized_path(path)
            require(not any(character in path for character in "*?[]"), "AI scope cannot contain wildcard paths.")
            require(not engine.protected_path(path), "AI scope cannot include protected paths.")
            scope.add(path)
        selected.append({key: item[key] for key in
                         ("id", "source_sha", "decision", "feature", "reason", "selected_scope", "ai_scope")})
    require(selected, "A source candidate requires at least one explicitly selected item.")
    return selected, scope


def metadata_paths(plan):
    return {"UPSTREAM.md", "VERSION", f"docs/upstream-sync/history/{plan['batch_id']}.json"}


def source_snapshot(repo, approval, revision, plan):
    """Reset only generated metadata to the approved tree and hash all other files."""
    with tempfile.TemporaryDirectory(prefix="upstream-handoff-index-") as directory:
        environment = dict(os.environ, GIT_INDEX_FILE=str(Path(directory) / "index"))
        engine.git(repo, "read-tree", revision, env=environment)
        for path in sorted(metadata_paths(plan)):
            value = blob(repo, approval, path, optional=True)
            if value is None:
                engine.git(repo, "update-index", "--force-remove", "--", path, env=environment)
            else:
                object_sha = engine.git(repo, "hash-object", "-w", "--stdin", input_data=value).stdout.decode().strip()
                engine.git(repo, "update-index", "--add", "--cacheinfo", "100644", object_sha, path, env=environment)
        tree = engine.git(repo, "write-tree", env=environment).stdout.decode().strip()
    patch = engine.git(repo, "diff", "--binary", "--full-index", "--no-ext-diff", "--no-renames", approval, tree).stdout
    return {"head": revision, "tree": tree, "patch_sha256": hashlib.sha256(patch).hexdigest(),
            "paths": changed_paths(repo, approval, tree)}


def make_record(repo, plan, plan_path, approval, runtime_head, revision, selected, scope):
    require(engine.is_ancestor(repo, approval, runtime_head), "Runtime code must descend from the original approval commit.")
    runtime = source_snapshot(repo, approval, runtime_head, plan)
    require(runtime["paths"] and set(runtime["paths"]) <= scope,
            "Runtime source changes must stay inside the exact human-approved file scope.")
    engine.check_patch_safety(repo, approval, runtime["tree"])
    engine.git(repo, "diff", "--check", approval, runtime["tree"])
    return {"schema_version": 1, "batch_id": plan["batch_id"], "mode": "manual_ai", "state": "ready_for_ci",
            "revision": revision, "integration": "selective", "approval_commit": approval,
            "plan_sha256": engine.plan_digest(plan), "plan_path": plan_path,
            "source": plan["source"], "downstream": plan["downstream"], "runtime": runtime,
            "selected_items": selected,
            "decisions": [{key: item[key] for key in ("id", "source_sha", "feature", "decision", "selected_scope",
                           "reason", "dependencies", "evidence", "patch", "ai_scope") if key in item} for item in plan["items"]]}


def metadata_files(repo, plan, approval, record):
    path = f"docs/upstream-sync/history/{plan['batch_id']}.json"
    upstream = blob(repo, approval, "UPSTREAM.md")
    appendix = f"\n## Manual AI synchronization batch `{plan['batch_id']}`\n\n"
    appendix += (f"Evaluated [{plan['source']['tag']}]({engine.SOURCE_REPOSITORY}/releases/tag/{plan['source']['tag']}) "
                 f"at `{plan['source']['sha']}`; range `{plan['source']['range_base_sha']}..{plan['source']['sha']}`. "
                 f"Initial fork baseline remains `{plan['source']['fork_baseline_sha']}`. This is selective integration.\n\n")
    appendix += (f"Human approval: `{approval}`; plan SHA-256: `{record['plan_sha256']}`. "
                 f"Downstream version: `{plan['downstream']['version']}` → `{plan['downstream']['proposed_version']}`. "
                 f"Revision {record['revision']} records source tree `{record['runtime']['tree']}` as **ready for CI**; "
                 "this record does not certify passing checks, merge or publication.\n\n")
    appendix += f"[Approved plan]({record['plan_path']}) · [Source and scope record]({path})\n"
    for item in record["selected_items"]:
        appendix += (f"\n- `{item['decision']}` [{item['source_sha']}]({engine.SOURCE_REPOSITORY}/commit/{item['source_sha']}): "
                     f"{item['feature']} — {item['reason']}\n")
    return {"UPSTREAM.md": upstream + appendix.encode("utf-8"),
            "VERSION": (plan["downstream"]["proposed_version"] + "\n").encode(), path: engine.canonical(record)}


def validate_metadata_history(repo, plan, plan_path, approval, head, selected, scope):
    """Excluded metadata must be an exact, append-only chain of generated revisions."""
    record_path = f"docs/upstream-sync/history/{plan['batch_id']}.json"
    cursor, revisions, latest = head, [], None
    while blob(repo, cursor, record_path, optional=True) is not None:
        record = json_blob(repo, cursor, record_path)
        require(isinstance(record, dict) and type(record.get("revision")) is int and record["revision"] > 0,
                "Invalid manual AI metadata revision.")
        runtime_head = record.get("runtime", {}).get("head")
        require(isinstance(runtime_head, str) and engine.SHA_RE.fullmatch(runtime_head)
                and runtime_head != cursor and engine.is_ancestor(repo, approval, runtime_head),
                "Invalid runtime source commit in metadata.")
        metadata_commit = engine.git_text(repo, "log", "-1", "--format=%H", cursor, "--", record_path)
        require(engine.git_text(repo, "show", "-s", "--format=%P", metadata_commit).split() == [runtime_head],
                "Generated metadata must directly follow its recorded runtime commit.")
        expected = make_record(repo, plan, plan_path, approval, runtime_head, record["revision"], selected, scope)
        require(record == expected, "History differs from the approved plan and exact runtime source proof.")
        files = metadata_files(repo, plan, approval, expected)
        require(set(changed_paths(repo, runtime_head, metadata_commit)) <= set(files),
                "The metadata commit cannot include source changes or unrelated files.")
        for path, content in files.items():
            require(blob(repo, metadata_commit, path, mode="100644") == content
                    and blob(repo, cursor, path, mode="100644") == content,
                    "Generated source/version/history metadata was modified or is not deterministic.")
        if latest is None:
            latest = (record, metadata_commit)
        revisions.append(record["revision"])
        cursor = runtime_head
    require(revisions == list(range(len(revisions), 0, -1)), "Metadata revisions must advance one at a time without replacement.")
    for path in metadata_paths(plan):
        require(blob(repo, cursor, path, optional=True, mode="100644")
                == blob(repo, approval, path, optional=True, mode="100644"),
                "Unrecorded edits to protected metadata cannot be excluded from source validation.")
    return latest


def read_plan(repo, plan_argument, head):
    path = Path(plan_argument)
    if path.is_absolute():
        require(path.resolve().is_relative_to(repo), "Plan must be inside its source checkout.")
        path = path.resolve().relative_to(repo)
    relative = normalized_path(path.as_posix())
    plan = json_blob(repo, head, relative)
    require(isinstance(plan, dict) and plan.get("schema_version") == 1 and plan.get("items"), "Invalid synchronization plan.")
    require(isinstance(plan.get("batch_id"), str) and engine.BATCH_RE.fullmatch(plan["batch_id"]), "Invalid batch ID.")
    context_paths(plan, relative)
    return plan, relative


def finalize(args):
    repo = Path(args.repo).resolve()
    engine.checkout_clean(repo)
    require(engine.git_text(repo, "branch", "--show-current").startswith("codex/upstream-plan-"),
            "Finalize on the existing upstream plan PR branch, never main or a detached checkout.")
    head = engine.resolve_commit(repo, "HEAD")
    plan, path = read_plan(repo, args.plan, head)
    selected, scope = validate_approval(repo, plan, path, args.approval_commit)
    previous = validate_metadata_history(repo, plan, path, args.approval_commit, head, selected, scope)
    if previous is not None and previous[1] == head:
        print(json.dumps({"state": "ready_for_ci", "batch_id": plan["batch_id"], "head": head,
                          "revision": previous[0]["revision"]}))
        return
    revision = previous[0]["revision"] + 1 if previous else 1
    record = make_record(repo, plan, path, args.approval_commit, head, revision, selected, scope)
    files = metadata_files(repo, plan, args.approval_commit, record)
    for relative, content in files.items():
        destination = engine.repository_path(repo, relative)
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(content)
    engine.git(repo, "add", "--", *sorted(files))
    engine.git(repo, "commit", "-m", f"docs: record manual upstream sync {plan['batch_id']} revision {revision}")
    print(json.dumps({"state": "ready_for_ci", "batch_id": plan["batch_id"], "head": engine.resolve_commit(repo, "HEAD"),
                      "revision": revision}))


def verify(args):
    repo = Path(args.repo).resolve()
    require(isinstance(args.head, str) and engine.SHA_RE.fullmatch(args.head)
            and engine.resolve_commit(repo, "HEAD") == args.head, "Checked out HEAD must match the exact PR event head.")
    engine.checkout_clean(repo)
    plan, path = read_plan(repo, args.plan, args.head)
    require(args.base == plan["downstream"]["base_sha"] and engine.is_ancestor(repo, args.base, args.head),
            "PR base changed; refresh and approve the synchronization plan against the current main base.")
    record_path = f"docs/upstream-sync/history/{plan['batch_id']}.json"
    if blob(repo, args.head, record_path, optional=True) is None:
        validate_plan_pins(repo, plan, args.head, planning=True)
        paths = changed_paths(repo, args.base, args.head)
        require(set(paths) <= context_paths(plan, path), "Planning-only PRs cannot include unapproved runtime source or release metadata.")
        for changed in paths:
            require(blob(repo, args.base, changed, optional=True) is None, "Planning cannot replace earlier batch files.")
            blob(repo, args.head, changed)
        print(json.dumps({"state": "planning", "batch_id": plan["batch_id"], "head": args.head}))
        return
    record = json_blob(repo, args.head, record_path)
    approval = record.get("approval_commit")
    selected, scope = validate_approval(repo, plan, path, approval)
    latest = validate_metadata_history(repo, plan, path, approval, args.head, selected, scope)
    require(latest is not None and latest[1] == args.head,
            "Candidate changed after metadata finalization; rerun finalize before CI can accept this HEAD.")
    print(json.dumps({"state": "ready_for_ci", "batch_id": plan["batch_id"], "head": args.head,
                      "revision": latest[0]["revision"]}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("finalize", "verify"):
        command = commands.add_parser(name)
        command.add_argument("--repo", default=".")
        command.add_argument("--plan", required=True)
        if name == "finalize":
            command.add_argument("--approval-commit", required=True)
        else:
            command.add_argument("--base", required=True)
            command.add_argument("--head", required=True)
    args = parser.parse_args()
    {"finalize": finalize, "verify": verify}[args.command](args)


if __name__ == "__main__":
    try:
        main()
    except (engine.SyncError, OSError, ValueError, TypeError, KeyError, subprocess.CalledProcessError) as error:
        print(f"upstream-sync handoff: {error}", file=sys.stderr)
        sys.exit(1)
