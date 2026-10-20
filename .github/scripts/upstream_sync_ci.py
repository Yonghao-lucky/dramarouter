#!/usr/bin/env python3
"""Trusted CI orchestration. Candidate code is never executed by publish."""

import argparse
import base64
import collections
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request

UPSTREAM = "https://github.com/QuantumNous/new-api"
CHECKS = {
    "frontend-types": ["bun", "run", "typecheck"],
    "frontend-tests": ["bun", "run", "test"],
    "frontend-build": ["bun", "run", "build"],
    "backend-tests": ["go", "test", "./..."],
    "backend-build": ["go", "build", "./..."],
    "relaykit-tests": ["go", "test", "./..."],
    "relaykit-independent-build": ["go", "build", "./..."],
}
HEX_SHA = re.compile(r"^[0-9a-f]{40}$")
BATCH = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
CONTROL_PARTS = {".git", ".github", ".agents", ".codex", ".aws"}
CONTROL_FILES = {"AGENTS.md", ".gitmodules", ".gitattributes", "LICENSE", "NOTICE", "THIRD-PARTY-LICENSES.md"}


def run(args, cwd=None, capture=False, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, check=True,
                            stdout=subprocess.PIPE if capture else None,
                            text=True)
    return result.stdout.strip() if capture else None


def git(repo, *args, capture=True, env=None):
    return run(["git", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false",
                "-C", str(repo), *args], capture=capture, env=env)


def origin(repo, *args, capture=True):
    environment = dict(os.environ)
    if environment.get("GH_TOKEN"):
        auth = base64.b64encode(("x-access-token:" + environment["GH_TOKEN"]).encode()).decode()
        environment.update(GIT_CONFIG_COUNT="1", GIT_CONFIG_KEY_0="http.extraheader",
                           GIT_CONFIG_VALUE_0="AUTHORIZATION: basic " + auth)
    return git(repo, *args, capture=capture, env=environment)


def require_sha(value):
    if not isinstance(value, str) or not HEX_SHA.fullmatch(value):
        raise ValueError("Expected a full lowercase commit SHA")
    return value


def require_batch(value):
    if not isinstance(value, str) or not BATCH.fullmatch(value):
        raise ValueError("Batch must use 1–64 lowercase letters, digits or hyphens")
    return value


def safe_path(value):
    if not isinstance(value, str) or not value or "\\" in value or any(ord(c) < 32 for c in value):
        raise ValueError("Unsafe repository path")
    path = PurePosixPath(value)
    if path.is_absolute() or any(part in ("", ".", "..") for part in value.split("/")):
        raise ValueError("Repository path must be relative and normalized")
    return path


def plan_file(root, value):
    path = safe_path(value)
    if len(path.parts) != 5 or path.parts[:3] != ("docs", "upstream-sync", "plans") or path.name != "plan.json":
        raise ValueError("Plan must be docs/upstream-sync/plans/<batch>/plan.json")
    require_batch(path.parts[3])
    full = root / str(path)
    if full.is_symlink() or not full.is_file() or not full.resolve().is_relative_to(root.resolve()):
        raise ValueError("Plan must be a regular file inside the planning checkout")
    return full


def read_json(path):
    if path.is_symlink() or path.stat().st_size > 10 * 1024 * 1024:
        raise ValueError("Unsafe or oversized JSON input")
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def core(trusted, *args, source=None):
    command = [sys.executable, "-c", source] if source else [sys.executable, str(trusted / ".github/scripts/upstream_sync.py")]
    result = subprocess.run([*command, *map(str, args)])
    if result.returncode:
        raise ValueError("Trusted upstream sync engine failed: " + str(args[0]))


def fetch_upstream(repo, target, range_base):
    if not isinstance(target, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/-]{0,127}", target):
        raise ValueError("Target must be an upstream tag name")
    run(["git", "check-ref-format", "refs/tags/" + target], capture=True)
    require_sha(range_base)
    # A differing existing tag is rejected; tags are never silently rewritten.
    git(repo, "fetch", "--no-tags", UPSTREAM, "refs/tags/" + target + ":refs/tags/" + target, capture=False)
    git(repo, "fetch", "--no-tags", UPSTREAM, range_base, capture=False)


def inspect_input(args):
    if args.mode == "analyze":
        require_batch(os.environ["BATCH"])
        require_sha(os.environ["RANGE_BASE"])
    else:
        ref = os.environ["PLAN_REF"]
        if not HEX_SHA.fullmatch(ref) and not re.fullmatch(r"codex/upstream-plan-[a-z0-9-]{1,120}", ref):
            raise ValueError("plan_ref must be an assessment branch or full commit SHA")
        path = safe_path(os.environ["PLAN_PATH"])
        if len(path.parts) != 5 or path.parts[:3] != ("docs", "upstream-sync", "plans") or path.name != "plan.json":
            raise ValueError("Invalid plan_path")
        require_batch(path.parts[3])


def analyze(args):
    source, trusted, out = args.source.resolve(), args.trusted.resolve(), args.output.resolve()
    batch = require_batch(os.environ["BATCH"])
    target, range_base = os.environ["TARGET"], os.environ["RANGE_BASE"]
    base = require_sha(git(source, "rev-parse", "HEAD"))
    fetch_upstream(source, target, range_base)
    destination = source / "docs/upstream-sync/plans" / batch
    if destination.exists():
        raise ValueError("Batch already exists; choose a new batch ID")
    core(trusted, "analyze", "--repo", source, "--source-repo", UPSTREAM,
         "--target", target, "--range-base", range_base, "--batch", batch,
         "--version", os.environ["NEXT_VERSION"], "--output-dir", destination)
    run([sys.executable, str(trusted / ".github/scripts/upstream_sync_handoff_context.py"),
         "--plan", str(destination / "plan.json"), "--output", str(destination / "AI_HANDOFF.md")])
    git(source, "add", "--", str(destination.relative_to(source)), capture=False)
    bundle(source, out, "analysis", batch, base, target)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as stream:
            stream.write(destination.joinpath("report.md").read_text(encoding="utf-8"))


def apply(args):
    source, trusted = args.source.resolve(), args.trusted.resolve()
    plan_root, candidate, out = args.plan_root.resolve(), args.candidate.resolve(), args.output.resolve()
    plan_path = plan_file(plan_root, os.environ["PLAN_PATH"])
    plan = read_json(plan_path)
    batch = require_batch(plan["batch_id"])
    if plan_path.parent.name != batch:
        raise ValueError("Plan batch and directory differ")
    base = require_sha(plan["downstream"]["base_sha"])
    # Execution starts from the reviewed default-branch commit, never the planning branch.
    if git(source, "rev-parse", "HEAD") != base:
        raise ValueError("Default branch advanced since assessment; regenerate/reassess the plan")
    if plan["source"]["repository"] != UPSTREAM:
        raise ValueError("Only the configured QuantumNous/new-api source is accepted")
    fetch_upstream(source, plan["source"]["tag"], plan["source"]["range_base_sha"])
    if git(source, "rev-parse", "refs/tags/" + plan["source"]["tag"] + "^{commit}") != require_sha(plan["source"]["sha"]):
        raise ValueError("Upstream target tag moved since assessment")
    core(trusted, "validate", "--repo", source, "--plan", plan_path, "--plan-root", plan_root)
    core(trusted, "apply", "--repo", source, "--plan", plan_path, "--plan-root", plan_root,
         "--candidate", candidate, "--result-dir", out)
    result_path = out / "result.json"
    result = read_json(result_path)
    if result["status"] != "applied" or result["base_sha"] != base:
        raise ValueError("Unexpected application result")
    verification_path = out / "verification.json"
    # Capture the executor before adopted build/test commands can access the workspace.
    engine_source = (trusted / ".github/scripts/upstream_sync.py").read_text(encoding="utf-8")
    verify_candidate(candidate, verification_path, plan["downstream"]["proposed_version"])
    core(trusted, "finalize", "--repo", candidate, "--plan", plan_path, "--plan-root", plan_root,
         "--result", result_path, "--verification", verification_path, source=engine_source)
    result = read_json(result_path)
    if result["status"] != "finalized":
        raise ValueError("Metadata finalization failed")
    bundle(candidate, out, "apply", batch, base, plan["source"]["tag"])


def verify_candidate(candidate, output, version=None):
    head = require_sha(git(candidate, "rev-parse", "HEAD"))
    env = {key: value for key, value in os.environ.items()
           if key in {"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "CI", "BUN_INSTALL", "GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE", "GOPROXY", "GOSUMDB", "SSL_CERT_FILE", "SSL_CERT_DIR"}}
    env.update(GOWORK="off", GIT_TERMINAL_PROMPT="0", CI="true")
    # No package-registry or repository write credential is present in this job.
    run(["bun", "install", "--frozen-lockfile"], cwd=candidate / "web", env=env)
    checks = []
    for name, command in CHECKS.items():
        directory = candidate / ("web" if name.startswith("frontend-") else "relaykit" if name.startswith("relaykit-") else "")
        if name == "frontend-build":
            env["VITE_REACT_APP_VERSION"] = version or (candidate / "VERSION").read_text().strip()
        print("::group::" + name, flush=True)
        try:
            run(command, cwd=directory, env=env)
        finally:
            print("::endgroup::", flush=True)
        checks.append({"name": name, "status": "passed"})
    if git(candidate, "rev-parse", "HEAD") != head or git(candidate, "status", "--porcelain", "--untracked-files=no"):
        raise ValueError("Validation changed tracked candidate files or HEAD")
    write_json(output, {"status": "passed", "validated_head": head, "checks": checks})


def bundle(repo, output, mode, batch, base, target):
    output.mkdir(parents=True, exist_ok=True)
    patch = subprocess.check_output(["git", "-C", str(repo), "diff", "--binary", "--full-index", "--no-ext-diff",
                                     "--no-renames", base, "--"] if mode == "analysis" else
                                    ["git", "-C", str(repo), "diff", "--binary", "--full-index", "--no-ext-diff",
                                     "--no-renames", base, "HEAD", "--"])
    if not patch or len(patch) > 50 * 1024 * 1024:
        raise ValueError("Empty or oversized publication patch")
    (output / "changes.patch").write_bytes(patch)
    candidate_head = None
    if mode == "apply":
        candidate_head = require_sha(git(repo, "rev-parse", "HEAD"))
        git(repo, "bundle", "create", str(output / "candidate.bundle"), "HEAD", "^" + base, capture=False)
    write_json(output / "publication.json", {"schema_version": 1, "mode": mode, "batch_id": require_batch(batch),
               "candidate_head": candidate_head,
               "base_sha": require_sha(base), "target": target,
               "patch_sha256": hashlib.sha256(patch).hexdigest(),
               "run_id": os.environ["GITHUB_RUN_ID"], "run_attempt": os.environ["GITHUB_RUN_ATTEMPT"]})


def allowed_path(value, mode, batch):
    path = safe_path(value)
    if mode == "analysis":
        return str(path) in {f"docs/upstream-sync/plans/{batch}/{name}"
                             for name in ("plan.json", "report.md", "AI_HANDOFF.md")}
    # Source changes to the executor, policy, attribution or git control files require a separate reviewed PR.
    return not any(part in CONTROL_PARTS for part in path.parts) and path.name not in CONTROL_FILES


def api(method, path, body=None):
    url = os.environ.get("GITHUB_API_URL", "https://api.github.com") + path
    request = urllib.request.Request(url, method=method, data=None if body is None else json.dumps(body).encode(),
                                     headers={"Authorization": "Bearer " + os.environ["GH_TOKEN"],
                                              "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"})
    try:
        with urllib.request.urlopen(request, timeout=45) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"GitHub API {method} failed ({error.code}). If PR creation is disabled, enable Settings → Actions → General → Allow GitHub Actions to create and approve pull requests.") from None


def publish(args):
    repo, output = args.source.resolve(), args.output.resolve()
    manifest = read_json(output / "publication.json")
    if manifest["schema_version"] != 1 or manifest["mode"] not in ("analysis", "apply"):
        raise ValueError("Invalid publication manifest")
    mode, batch = manifest["mode"], require_batch(manifest["batch_id"])
    for name in ("run_id", "run_attempt"):
        if manifest[name] != os.environ["GITHUB_" + name.upper()]:
            raise ValueError("Artifact does not belong to the current workflow run/attempt")
    base = require_sha(manifest["base_sha"])
    default_branch = os.environ["DEFAULT_BRANCH"]
    git(repo, "check-ref-format", "refs/heads/" + default_branch)
    if not args.dry_run:
        origin(repo, "fetch", "--no-tags", "origin", "refs/heads/" + default_branch, capture=False)
    if git(repo, "rev-parse", "HEAD" if args.dry_run else "FETCH_HEAD") != base:
        raise ValueError("Default branch advanced; reassess instead of publishing a stale plan")
    git(repo, "reset", "--mixed", base, capture=False)
    patch_path = output / "changes.patch"
    if patch_path.is_symlink() or patch_path.stat().st_size > 50 * 1024 * 1024:
        raise ValueError("Unsafe publication patch")
    if hashlib.sha256(patch_path.read_bytes()).hexdigest() != manifest["patch_sha256"]:
        raise ValueError("Publication patch digest mismatch")
    # Apply to the index only. Never check out, import or execute the adopted source with a write token.
    git(repo, "apply", "--cached", "--check", "--", str(patch_path), capture=False)
    git(repo, "apply", "--cached", "--", str(patch_path), capture=False)
    names = subprocess.check_output(["git", "-C", str(repo), "diff", "--cached", "--name-only", "-z", "--no-renames"]).decode().split("\0")
    names = [name for name in names if name]
    if not names or any(not allowed_path(name, mode, batch) for name in names):
        raise ValueError("Patch touches a protected control, attribution or out-of-scope path")
    for name in names:
        record = git(repo, "ls-files", "--stage", "--", name)
        if record and record.split()[0] != "100644" and record.split()[0] != "100755":
            raise ValueError("Symlink and submodule changes are not permitted")
    if mode == "apply" and not {"VERSION", "UPSTREAM.md", f"docs/upstream-sync/history/{batch}.json"}.issubset(names):
        raise ValueError("Implementation patch must update source, version and history records")
    tree = git(repo, "write-tree")
    candidate_head = None
    if mode == "apply":
        candidate_head = require_sha(manifest["candidate_head"])
        bundle_path = output / "candidate.bundle"
        if bundle_path.is_symlink() or bundle_path.stat().st_size > 100 * 1024 * 1024:
            raise ValueError("Unsafe or oversized candidate bundle")
        git(repo, "bundle", "verify", str(bundle_path), capture=False)
        git(repo, "fetch", "--no-tags", str(bundle_path), "HEAD:refs/upstream-sync/candidate", capture=False)
        if git(repo, "rev-parse", "refs/upstream-sync/candidate") != candidate_head:
            raise ValueError("Candidate bundle HEAD differs from the pinned manifest")
        if git(repo, "show", "-s", "--format=%T", candidate_head) != tree:
            raise ValueError("Candidate bundle and publication patch have different trees")
        git(repo, "merge-base", "--is-ancestor", base, candidate_head)
        # Drop the adopted index before validating provenance against the trusted base checkout.
        git(repo, "read-tree", base, capture=False)
        with tempfile.TemporaryDirectory(prefix="upstream-reviewed-") as directory:
            plan_root = Path(directory)
            relative_plan = f"docs/upstream-sync/plans/{batch}/plan.json"
            if os.environ.get("PLAN_PATH") != relative_plan:
                raise ValueError("Published batch differs from the dispatched plan path")
            inspect_input(argparse.Namespace(mode="apply"))
            plan_ref = os.environ["PLAN_REF"]
            origin(repo, "fetch", "--no-tags", "origin", plan_ref if HEX_SHA.fullmatch(plan_ref) else "refs/heads/" + plan_ref, capture=False)
            reviewed_commit = require_sha(git(repo, "rev-parse", "FETCH_HEAD"))
            plan_bytes = subprocess.check_output(["git", "-C", str(repo), "show", reviewed_commit + ":" + relative_plan])
            plan = json.loads(plan_bytes)
            bundled_plan = json.loads(subprocess.check_output(["git", "-C", str(repo), "show", candidate_head + ":" + relative_plan]))
            if plan != bundled_plan:
                raise ValueError("Published decisions differ from the reviewed planning branch")
            destination = plan_root / relative_plan
            destination.parent.mkdir(parents=True)
            destination.write_bytes(plan_bytes)
            if plan.get("source", {}).get("repository") != UPSTREAM:
                raise ValueError("Candidate plan uses an unexpected source repository")
            fetch_upstream(repo, plan["source"]["tag"], plan["source"]["range_base_sha"])
            for item in plan.get("items", []):
                if item.get("decision") == "migrate":
                    relative = safe_path(item["patch"]["path"])
                    if not str(relative).startswith("docs/upstream-sync/patches/") or relative.suffix != ".patch":
                        raise ValueError("Unexpected migration patch location")
                    destination = plan_root / str(relative)
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_bytes(subprocess.check_output(["git", "-C", str(repo), "show", reviewed_commit + ":" + str(relative)]))
            result = read_json(output / "result.json")
            if result.get("metadata_commit") != candidate_head or result.get("batch_id") != batch:
                raise ValueError("Execution result does not identify the bundled candidate")
            core(repo, "verify-candidate", "--repo", repo, "--plan", plan_root / relative_plan,
                 "--plan-root", plan_root, "--result", output / "result.json", "--verification", output / "verification.json")
    identity_name = os.environ["GIT_AUTHOR_NAME"]
    identity_email = os.environ["GIT_AUTHOR_EMAIL"]
    authors = git(repo, "log", "-n", "2000", "--format=%aN <%aE>", base).splitlines()
    core_authors = {author for author, _ in collections.Counter(authors).most_common(10)}
    assisted = f"{identity_name} <{identity_email}>" not in core_authors
    kind = "plan" if mode == "analysis" else "apply"
    branch = f"codex/upstream-{kind}-{batch}-{manifest['run_id']}-{manifest['run_attempt']}"
    title = f"chore(upstream): {'WIP' if mode == 'analysis' else 'sync'} {batch}"
    if args.dry_run:
        print(json.dumps({"status": "validated", "mode": mode, "batch_id": batch, "tree": tree, "candidate_head": candidate_head}))
        return
    commit = candidate_head or git(repo, "commit-tree", tree, "-p", base, "-m", title)
    # Authenticate only operations against the trusted checkout origin; never upstream fetches.
    existing_ref = origin(repo, "ls-remote", "--heads", "origin", "refs/heads/" + branch)
    if existing_ref:
        existing_commit = require_sha(existing_ref.split()[0])
        origin(repo, "fetch", "--no-tags", "origin", "refs/heads/" + branch, capture=False)
        if mode == "apply":
            if existing_commit != commit:
                raise ValueError("Publication branch already points to a different candidate")
        elif git(repo, "show", "-s", "--format=%T %P", existing_commit) != tree + " " + base:
            raise ValueError("Planning branch already contains a different tree or parent")
        commit = existing_commit
    else:
        origin(repo, "push", "origin", commit + ":refs/heads/" + branch, capture=False)
    template = (repo / ".github/PULL_REQUEST_TEMPLATE.md").read_text(encoding="utf-8")
    run_url = f"{os.environ['GITHUB_SERVER_URL']}/{os.environ['GITHUB_REPOSITORY']}/actions/runs/{manifest['run_id']}"
    description = (f"Start upstream `{manifest['target']}` synchronization in this Draft/WIP PR. "
                   "The maintainer manually asks an AI agent to analyze the differences, confirms the selected plan, then asks the agent to implement on this same branch. "
                   "Keep this PR in draft until implementation, source/version records and current-HEAD CI checks are complete. "
                   "CI does not call an AI model; Ready for review and merge remain explicit maintainer actions." if mode == "analysis" else
                   f"Apply the selected upstream items from batch `{batch}`. The candidate passed frontend type checks, tests and production build, root Go tests/build and independent relaykit tests/build before publication. Review the plan, source mappings and history before merging manually.")
    template = template.replace("(简述：做了什么？为什么这样改能生效？请基于你对代码逻辑的理解来写，避免粘贴未经整理的内容)", description)
    template = template.replace("- [ ] 📝 文档更新 (Documentation)", "- [x] 📝 文档更新 (Documentation)")
    template = template.replace("- Closes # (如有)", f"- Sync batch: `{batch}`; planning file: `docs/upstream-sync/plans/{batch}/plan.json`.")
    template = template.replace("- 与上游关系：上游已包含 / 跟踪上游 Issue 或 PR / NovaMaaS 下游专属 / 不适用（说明原因）", f"- 与上游关系：选择性评估／同步 QuantumNous/new-api `{manifest['target']}`；初始 fork 基线保持不变。")
    template = template.replace("- 差异摘要：（如形成需要长期维护的核心运行时差异，请说明是否需要更新 README 关键差异表）", "- 差异摘要：维护者应核对运行时行为，并按 README 台账标准人工决定是否补充长期差异。")
    template = template.replace("(请在此粘贴截图、关键日志或测试报告，以证明变更生效)", f"- Workflow evidence: {run_url}\n- Patch SHA-256: `{manifest['patch_sha256']}`")
    template += ("\n\nAutomation disclosure: This draft PR and its source changes were AI-assisted and require human review. "
                 f"The automation identity was compared with recurring historical authors (historical core identity: {'yes' if not assisted else 'no'}). "
                 "Human confirmation checkboxes are intentionally unchecked. No automatic merge is configured.\n")
    repository = os.environ["GITHUB_REPOSITORY"]
    owner = repository.split("/", 1)[0]
    if mode == "analysis":
        handoff_url = (f"{os.environ['GITHUB_SERVER_URL']}/{repository}/blob/{branch}/"
                       f"docs/upstream-sync/plans/{batch}/AI_HANDOFF.md")
        template += (f"\n## AI 接管与进度\n\n接管说明：[AI_HANDOFF.md]({handoff_url})\n\n"
                     "- [ ] AI 已分析差异并给出功能建议\n"
                     "- [ ] 维护者已确认计划与允许修改的范围，并记录确认提交 SHA\n"
                     "- [ ] AI 已在本 PR 分支完成适配、测试及来源和版本记录\n"
                     "- [ ] 当前 PR HEAD 的 CI 已通过\n"
                     "- [ ] 维护者已完成审查，可转为 Ready for review\n\n"
                     f"AI 接管第一步：检出本 PR 分支，先阅读 AGENTS.md、UPSTREAM.md 和 "
                     f"docs/upstream-sync/plans/{batch}/AI_HANDOFF.md，分析并更新建议；"
                     "保持 decision=pending，等待我确认。后续继续在本 PR 提交，禁止自动合并 main。\n")
    existing = api("GET", f"/repos/{repository}/pulls?state=open&head=" + urllib.parse.quote(owner + ":" + branch, safe=""))
    pull = existing[0] if existing else api("POST", f"/repos/{repository}/pulls", {"title": title, "body": template, "head": branch,
                                                                                 "base": default_branch, "draft": True})
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as stream:
            stream.write(f"### Upstream sync PR\n\n[{title}]({pull['html_url']})\n\nManual review and merge are required.\n")


def revalidate(args):
    candidate, trusted = args.candidate.resolve(), args.trusted.resolve()
    base = require_sha(os.environ["PR_BASE_SHA"])
    head = require_sha(os.environ["PR_HEAD_SHA"])
    if git(candidate, "rev-parse", "HEAD") != head:
        raise ValueError("PR HEAD does not match the checked-out commit")
    if os.environ.get("PR_HEAD_REF", "").startswith("codex/upstream-plan-"):
        plans = git(candidate, "diff", "--name-only", "--diff-filter=AM", base, head,
                    "--", "docs/upstream-sync/plans").splitlines()
        plans = [name for name in plans if re.fullmatch(r"docs/upstream-sync/plans/[a-z0-9-]+/plan\.json", name)]
        if len(plans) != 1:
            raise ValueError("WIP PR must retain exactly one synchronization plan")
        plan_path = plan_file(candidate, plans[0])
        plan = read_json(plan_path)
        if plan.get("source", {}).get("repository") != UPSTREAM:
            raise ValueError("WIP plan uses an unexpected source repository")
        fetch_upstream(candidate, plan["source"]["tag"], plan["source"]["range_base_sha"])
        verdict = json.loads(run([sys.executable, str(trusted / ".github/scripts/upstream_sync_handoff.py"),
                                  "verify", "--repo", str(candidate), "--plan", str(plan_path),
                                  "--base", base, "--head", head], capture=True))
        if verdict["state"] == "planning":
            if os.environ.get("PR_DRAFT") != "true":
                raise ValueError("Planning-only PR must remain Draft; complete AI implementation and records before Ready for review")
            write_json(args.output.resolve() / "verification.json",
                       {"status": "planning", "validated_head": head, "checks": [],
                        "message": "Waiting for maintainer-confirmed AI implementation; not merge-ready."})
            return
        if verdict["state"] != "ready_for_ci":
            raise ValueError("Unexpected WIP handoff state")
        verify_candidate(candidate, args.output.resolve() / "verification.json")
        return
    names = git(candidate, "diff", "--name-only", base, head, "--", "docs/upstream-sync/history").splitlines()
    records = [name for name in names if re.fullmatch(r"docs/upstream-sync/history/[a-z0-9-]+\.json", name)]
    if not records and os.environ.get("PR_HEAD_REF", "").startswith("codex/upstream-apply-"):
        raise ValueError("Generated implementation PR must retain its synchronization history")
    if len(records) != 1:
        raise ValueError("Implementation PR must introduce exactly one synchronization batch")
    path = candidate / records[0]
    record = read_json(path)
    batch = require_batch(record["batch_id"])
    relative_plan = f"docs/upstream-sync/plans/{batch}/plan.json"
    plan = plan_file(candidate, relative_plan)
    pinned_base = require_sha(record["downstream"]["base_sha"])
    git(trusted, "merge-base", "--is-ancestor", pinned_base, base)
    fetch_upstream(trusted, record["source"]["tag"], record["source"]["range_base_sha"])
    git(trusted, "fetch", "--no-tags", str(candidate), head, capture=False)
    with tempfile.TemporaryDirectory(prefix="upstream-pr-record-") as directory:
        temporary = Path(directory)
        verification_base = temporary / "base"
        git(trusted, "worktree", "add", "--detach", str(verification_base), pinned_base, capture=False)
        try:
            result = {"schema_version": 1, "status": "finalized", "batch_id": batch,
                      "base_sha": pinned_base, "candidate_head": record["candidate_head"],
                      "plan_sha256": record["plan_sha256"], "mappings": record["mappings"],
                      "metadata_commit": head, "record_path": records[0]}
            write_json(temporary / "result.json", result)
            write_json(temporary / "verification.json", record["verification"])
            core(trusted, "verify-candidate", "--repo", verification_base, "--plan", plan,
                 "--plan-root", candidate, "--result", temporary / "result.json",
                 "--verification", temporary / "verification.json")
        finally:
            git(trusted, "worktree", "remove", "--force", str(verification_base), capture=False)
    verify_candidate(candidate, args.output.resolve() / "verification.json")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("inspect-input", "analyze", "apply", "publish", "revalidate"))
    parser.add_argument("--mode", choices=("analyze", "apply"))
    parser.add_argument("--trusted", type=Path)
    parser.add_argument("--source", type=Path)
    parser.add_argument("--plan-root", type=Path)
    parser.add_argument("--candidate", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--dry-run", action="store_true", help="Validate a publication bundle without pushing or opening a PR (source and plan references may be fetched)")
    args = parser.parse_args()
    {"inspect-input": inspect_input, "analyze": analyze, "apply": apply, "publish": publish, "revalidate": revalidate}[args.command](args)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError, RuntimeError) as error:
        print(f"upstream-sync CI: {error}", file=sys.stderr)
        sys.exit(1)
