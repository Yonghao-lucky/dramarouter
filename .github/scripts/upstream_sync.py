#!/usr/bin/env python3
"""Review and selectively integrate pinned upstream releases (Python stdlib only)."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


SOURCE_REPOSITORY = "https://github.com/QuantumNous/new-api"
DECISIONS = {"pending", "merge", "migrate", "upgrade", "skip", "defer", "covered"}
ADOPTED = {"merge", "migrate", "upgrade"}
REQUIRED_CHECKS = {
    "frontend-types", "frontend-tests", "frontend-build", "backend-tests",
    "backend-build", "relaykit-tests", "relaykit-independent-build",
}
SHA_RE = re.compile(r"[0-9a-f]{40}\Z")
VERSION_RE = re.compile(r"v1\.0\.0-rc\.26-novamaas\.([0-9]+)\Z")
BATCH_RE = re.compile(r"[a-z0-9][a-z0-9-]{0,63}\Z")
PROTECTED_REFERENCE = re.compile(r"new[-_ ]api|quantumnous|nеw-аρi|quаntumνоuѕ", re.IGNORECASE)


class SyncError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise SyncError(message)


def git(repo, *args, input_data=None, check=True, env=None):
    """Never invoke a shell or repository hooks; return raw bytes."""
    command = ["git", "-c", "core.hooksPath=/dev/null", "-C", str(repo), *args]
    result = subprocess.run(command, input=input_data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    if check and result.returncode:
        raise SyncError(f"git {' '.join(args[:3])} failed: {result.stderr.decode(errors='replace').strip()}")
    return result


def git_text(repo, *args):
    return git(repo, *args).stdout.decode("utf-8", errors="replace").strip()


def canonical(value):
    return (json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")


def read_json(path):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise SyncError(f"Cannot read JSON {path}: {error}") from error


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(canonical(value))


def plan_digest(plan):
    return hashlib.sha256(canonical(plan)).hexdigest()


def checkout_clean(repo):
    require(not git_text(repo, "status", "--porcelain", "--untracked-files=all"),
            "Checkout must be clean, including untracked files; commit the plan in a separate checkout.")


def fork_baseline(repo):
    text = (Path(repo) / "UPSTREAM.md").read_text(encoding="utf-8")
    match = re.search(r"^- Commit / 原始提交：`([0-9a-f]{40})`", text, re.MULTILINE)
    require(match is not None, "UPSTREAM.md must preserve its initial Commit / 原始提交 field.")
    return match.group(1)


def resolve_commit(repo, ref):
    require(isinstance(ref, str) and ref and not ref.startswith("-") and "\x00" not in ref,
            "Invalid Git reference.")
    return git_text(repo, "rev-parse", "--verify", f"{ref}^{{commit}}")


def is_ancestor(repo, older, newer):
    return git(repo, "merge-base", "--is-ancestor", older, newer, check=False).returncode == 0


def range_commits(repo, base, target):
    return git_text(repo, "rev-list", "--reverse", "--topo-order", f"{base}..{target}").splitlines()


def commit_info(repo, sha):
    parents = git_text(repo, "show", "-s", "--format=%P", sha).split()
    require(parents, "Upstream source commits must have a parent.")
    files = git(repo, "diff", "--name-only", "--no-renames", "-z", parents[0], sha).stdout
    return {
        "id": "commit-" + sha[:12], "source_sha": sha,
        "title": git_text(repo, "show", "-s", "--format=%s", sha),
        "files": sorted(item.decode("utf-8") for item in files.split(b"\x00") if item),
        "parents": parents,
    }


def patch_ids(repo, revision_range):
    patches = git(repo, "log", "--no-merges", "--format=medium", "-p", revision_range).stdout
    lines = git(repo, "patch-id", "--stable", input_data=patches).stdout.decode().splitlines()
    return {parts[1]: parts[0] for line in lines if len(parts := line.split()) == 2}


def protected_path(path):
    parts = Path(path).parts
    if not parts or any(part.lower() in {".git", ".github", ".agents", ".codex", ".aws"} for part in parts):
        return True
    name = parts[-1].upper()
    return (name in {"AGENTS.MD", "UPSTREAM.MD", "VERSION", ".GITMODULES", ".GITATTRIBUTES"}
            or name.startswith(("README", "LICENSE", "LICENCE", "NOTICE", "COPYING", "THIRD-PARTY-LICENSE"))
            or path.lower().startswith("docs/upstream-sync/"))


def check_patch_safety(repo, before, after=None, cached=False):
    args = ["diff", "--no-ext-diff", "--no-renames"]
    if cached:
        args.append("--cached")
    args.append(before)
    if after:
        args.append(after)
    names = git(repo, *args, "--name-only", "-z").stdout
    paths = [item.decode("utf-8") for item in names.split(b"\x00") if item]
    blocked = [path for path in paths if protected_path(path)]
    require(not blocked, "Automatic application cannot modify protected paths: " + ", ".join(blocked))
    diff = git(repo, *args, "--unified=0").stdout.decode("utf-8", errors="replace")
    removed = [line[1:] for line in diff.splitlines() if line.startswith("-") and not line.startswith("---")]
    require(not any(PROTECTED_REFERENCE.search(line) for line in removed),
            "Automatic application cannot remove or modify existing lines mentioning protected project identities.")
    modes = git(repo, *args, "--raw").stdout.decode("utf-8", errors="replace")
    require(not any({"160000", "120000"} & set(line.split("\t")[0].lstrip(":").split()[:2])
                    for line in modes.splitlines()), "Automatic application cannot modify symlinks or submodules.")


def migration_safety(repo, content):
    """Inspect the reviewed patch before creating a candidate or writing its files."""
    names = git(repo, "apply", "--numstat", "-z", "-", input_data=content).stdout
    paths = []
    for record in names.split(b"\x00"):
        if record:
            fields = record.split(b"\t", 2)
            require(len(fields) == 3, "Migration patch contains unsupported rename metadata.")
            paths.append(fields[2].decode("utf-8"))
    require(paths and not any(protected_path(path) for path in paths),
            "Migration patch cannot modify protected paths.")
    text = content.decode("utf-8", errors="replace")
    removed = [line[1:] for line in text.splitlines() if line.startswith("-") and not line.startswith("---")]
    require(not any(PROTECTED_REFERENCE.search(line) for line in removed),
            "Migration patch cannot remove or modify protected project identities.")
    require(not re.search(r"^(?:new file mode|deleted file mode|old mode|new mode) (?:120000|160000)$", text, re.MULTILINE),
            "Migration patch cannot modify symlinks or submodules.")


def history_records(repo):
    records = []
    directory = Path(repo) / "docs/upstream-sync/history"
    if directory.exists():
        for path in sorted(directory.glob("*.json")):
            record = read_json(path)
            require(isinstance(record, dict), f"Invalid history record: {path}")
            records.append(record)
    return records


def historical_evidence(records, evaluated_shas):
    prior = {}
    latest = {}
    ordered = sorted(records, key=lambda record: (
        int(match.group(1)) if (match := VERSION_RE.fullmatch(record.get("downstream", {}).get("proposed_version", ""))) else -1,
        record.get("batch_id", ""),
    ))
    for record in ordered:
        for decision in record.get("decisions", []):
            sha = decision.get("source_sha")
            require(isinstance(sha, str) and SHA_RE.fullmatch(sha), "Historical decisions require full source SHAs.")
            evidence = {"batch_id": record["batch_id"], "source_sha": sha,
                        "decision": decision["decision"], "selected_scope": decision.get("selected_scope", "full_commit"),
                        "feature": decision.get("feature", ""), "reason": decision.get("reason", "")}
            prior.setdefault(sha, []).append({key: evidence[key] for key in
                                             ("batch_id", "decision", "selected_scope", "reason")})
            latest[sha] = evidence
    follow_up = [entry for sha, entry in sorted(latest.items()) if sha not in evaluated_shas
                 and (entry["decision"] in {"skip", "defer"} or entry["selected_scope"] in {"reviewed_patch", "ai_adaptation"})]
    return prior, follow_up


def render_report(plan):
    source = plan["source"]
    downstream = plan["downstream"]
    lines = [
        f"# Upstream synchronization assessment: {plan['batch_id']}", "",
        "This report contains mechanical evidence, any recorded AI advice and review prompts. Advice does not establish semantic compatibility or authorize implementation.", "",
        f"- Source: {source['repository']}",
        f"- Target release: `{source['tag']}` (`{source['sha']}`)",
        f"- Evaluated range: `{source['range_base_sha']}..{source['sha']}`",
        f"- Preserved initial fork baseline: `{source['fork_baseline_sha']}`",
        f"- Downstream base: `{downstream['base_sha']}` (`{downstream['version']}`)",
        f"- Proposed downstream version: `{downstream['proposed_version']}`", "",
        "Each source commit is a review item, not a proven feature boundary. Use feature/group names and dependencies to describe cross-commit work.",
        "All decisions need a reason. `covered` also needs evidence; patch equivalence and old source references are evidence only.",
        "`merge` and `upgrade` use source commits; approved AI scopes permit conflict adaptation. `migrate` uses a reviewed patch or an explicitly approved AI scope; no commands run from the plan.",
        "Sensitive CI/policy/license/provenance/version paths, symlinks, submodules and changes to existing attribution lines cannot be applied automatically.", "",
        "## Historical follow-up outside this evaluated range", "",
        "Previous skipped/deferred sources and partial migrations stay visible here. These are review evidence, not selected work; a later full adoption or documented coverage supersedes them.", "",
    ]
    follow_up = plan.get("historical_follow_up", [])
    if follow_up:
        lines += ["| Source | Previous batch | Decision / scope | Reason |", "| --- | --- | --- | --- |"]
        for entry in follow_up:
            reason = entry["reason"].replace("|", "\\|").replace("\n", " ")
            lines.append(f"| `{entry['source_sha']}` | {entry['batch_id']} | {entry['decision']} / {entry['selected_scope']} | {reason} |")
    else:
        lines.append("No historical follow-up outside this range is recorded.")
    lines += ["", "## Current evaluated range", "",
        "| Item | Source | Decision | Files | Downstream overlap | Review signals |",
        "| --- | --- | --- | ---: | ---: | --- |",
    ]
    for item in plan["items"]:
        analysis = item.get("analysis", {})
        signals = []
        if analysis.get("known_source_reference"):
            signals.append("previous source reference")
        if analysis.get("equivalent_patch_commits"):
            signals.append("equivalent patch")
        if analysis.get("protected_files"):
            signals.append("protected files")
        if len(item["parents"]) > 1:
            signals.append("merge commit: cannot cherry-pick automatically")
        label = item["id"].replace("|", "\\|")
        lines.append(f"| {label} | [{item['source_sha'][:12]}]({source['repository']}/commit/{item['source_sha']}) | {item['decision']} | {len(item['files'])} | {len(analysis.get('downstream_overlap', []))} | {'; '.join(signals) or 'review required'} |")
    for item in plan["items"]:
        analysis = item.get("analysis", {})
        lines += ["", f"## {item['id']}", "", f"- Title: {item['title']}",
                  f"- Feature/group: {item.get('feature', '')}", f"- Decision: `{item['decision']}`",
                  f"- Selected scope: `{item.get('selected_scope', 'full_commit')}`",
                  f"- Reason: {item.get('reason', '')}",
                  f"- Dependencies: {', '.join(item.get('dependencies', [])) or 'none declared; requires review'}",
                  f"- Files: {', '.join('`' + path + '`' for path in item['files']) or '(none)'}",
                  f"- Overlap with downstream changes: {', '.join(analysis.get('downstream_overlap', [])) or '(none detected)'}",
                  f"- Equivalent downstream patch IDs: {', '.join(analysis.get('equivalent_patch_commits', [])) or '(none detected)'}",
                  f"- Existing source reference: {analysis.get('known_source_reference', False)}",
                  "- Review behavior, protocol/billing/auth boundaries, all database dialects, dependencies and regression coverage."]
        if item.get("evidence"):
            lines.append("- Decision evidence: " + "; ".join(item["evidence"]))
        if item.get("ai_scope"):
            lines.append("- Approved AI paths: " + ", ".join(item["ai_scope"]["paths"]))
            lines.append("- Approved AI instructions: " + item["ai_scope"]["instructions"])
        if item.get("ai_analysis"):
            assessment = item["ai_analysis"]
            lines += [f"- AI feature/group: {assessment['feature']}",
                      f"- AI recommendation: `{assessment['recommendation']}` — {assessment['reason']}",
                      "- AI risks: " + "; ".join(assessment["risks"]),
                      "- AI dependencies: " + (", ".join(assessment["dependencies"]) or "none identified"),
                      "- AI suggested paths (not approval): " + ", ".join(assessment["suggested_allowed_paths"])]
            for evidence in assessment["evidence"]:
                lines.append(f"- AI evidence: `{evidence['revision']}:{evidence['path']}:{evidence['line']}` — {evidence['detail']}")
        for previous in analysis.get("prior_decisions", []):
            lines.append(f"- Prior batch `{previous['batch_id']}`: `{previous['decision']}` / `{previous['selected_scope']}` — {previous['reason']}")
    return "\n".join(lines) + "\n"


def analyze(args):
    repo = Path(args.repo).resolve()
    checkout_clean(repo)
    require(args.source_repo.rstrip("/").removesuffix(".git") == SOURCE_REPOSITORY, "Only the official pinned upstream repository is supported.")
    require(BATCH_RE.fullmatch(args.batch), "Batch IDs must contain 1–64 lowercase letters, digits or hyphens.")
    git(repo, "check-ref-format", f"refs/tags/{args.target}")
    target = resolve_commit(repo, f"refs/tags/{args.target}")
    base = resolve_commit(repo, args.range_base)
    baseline = fork_baseline(repo)
    downstream_sha = resolve_commit(repo, "HEAD")
    require(is_ancestor(repo, baseline, base) and is_ancestor(repo, base, target), "Evaluation range must descend from the initial fork baseline.")
    require(is_ancestor(repo, baseline, downstream_sha), "Downstream checkout must retain initial fork history.")
    version = (repo / "VERSION").read_text().strip()
    validate_version(version, args.version)
    records = history_records(repo)
    require(not any(record.get("batch_id") == args.batch for record in records), "Batch has already been applied; use a new batch ID.")
    shas = range_commits(repo, base, target)
    require(shas, "Selected evaluation range has no commits.")
    prior_decisions, follow_up = historical_evidence(records, set(shas))
    downstream_files = set(git_text(repo, "diff", "--name-only", "--no-renames", baseline, downstream_sha).splitlines())
    old_text = (repo / "UPSTREAM.md").read_text()
    old_text += "\n" + "\n".join(json.dumps(record) for record in records)
    downstream_patches = patch_ids(repo, f"{baseline}..{downstream_sha}")
    source_patches = patch_ids(repo, f"{base}..{target}")
    items = []
    for sha in shas:
        info = commit_info(repo, sha)
        info.update({
            "feature": info["title"], "decision": "pending", "reason": "", "dependencies": [],
            "selected_scope": "full_commit",
            "patch": None, "evidence": [],
            "analysis": {
                "downstream_overlap": sorted(set(info["files"]) & downstream_files),
                "protected_files": [path for path in info["files"] if protected_path(path)],
                "known_source_reference": sha in old_text,
                "prior_decisions": prior_decisions.get(sha, []),
                "equivalent_patch_commits": sorted(commit for commit, patch_id in downstream_patches.items()
                                                   if source_patches.get(sha) == patch_id),
            },
        })
        items.append(info)
    plan = {
        "schema_version": 1, "batch_id": args.batch,
        "source": {"repository": SOURCE_REPOSITORY, "tag": args.target, "sha": target,
                   "range_base_sha": base, "fork_baseline_sha": baseline},
        "downstream": {"base_sha": downstream_sha, "version": version, "proposed_version": args.version},
        "historical_follow_up": follow_up,
        "items": items,
    }
    directory = Path(args.output_dir)
    require(not directory.exists(), "Analysis output directory already exists; refusing to overwrite a reviewed plan.")
    write_json(directory / "plan.json", plan)
    (directory / "report.md").write_text(render_report(plan), encoding="utf-8")
    print(json.dumps({"plan": str(directory / "plan.json"), "report": str(directory / "report.md"), "items": len(items)}))


def validate_version(current, proposed):
    old, new = VERSION_RE.fullmatch(current), VERSION_RE.fullmatch(proposed)
    require(old is not None and new is not None and int(new.group(1)) > int(old.group(1)),
            "Proposed version must retain v1.0.0-rc.26-novamaas.N with a strictly increasing integer.")


def repository_path(root, relative):
    root = Path(root).resolve()
    require(isinstance(relative, str) and not Path(relative).is_absolute()
            and ".." not in Path(relative).parts, "Repository paths cannot escape their checkout.")
    current = root
    parts = Path(relative).parts
    for index, part in enumerate(parts):
        current /= part
        require(not current.is_symlink(), "Repository paths cannot traverse symlinks.")
        require(not current.exists() or (current.is_file() if index == len(parts) - 1 else current.is_dir()),
                "Repository path collides with a file or directory of a different kind.")
    require(current.resolve().is_relative_to(root), "Repository path escaped its checkout.")
    return current


def migration_patch(item, plan_root):
    patch = item.get("patch")
    require(isinstance(patch, dict) and set(patch) == {"path", "sha256"}, "migrate requires patch {path, sha256}.")
    relative = patch["path"]
    require(isinstance(relative, str) and relative.startswith("docs/upstream-sync/patches/")
            and relative.endswith(".patch") and ".." not in Path(relative).parts,
            "Migration patches must be repository-relative docs/upstream-sync/patches/*.patch files.")
    path = repository_path(plan_root, relative)
    require(path.is_file(), "Migration patch must be a regular file inside plan root.")
    content = path.read_bytes()
    require(hashlib.sha256(content).hexdigest() == patch["sha256"], "Migration patch SHA-256 does not match the reviewed plan.")
    return content


def validate_plan(repo, plan_path, plan_root=None, require_base=True, check_clean=True):
    repo = Path(repo).resolve()
    plan = read_json(plan_path)
    require(isinstance(plan, dict) and plan.get("schema_version") == 1, "Unsupported plan schema_version.")
    require(isinstance(plan.get("batch_id"), str) and BATCH_RE.fullmatch(plan["batch_id"]), "Invalid batch ID.")
    source, downstream = plan.get("source", {}), plan.get("downstream", {})
    require(source.get("repository") == SOURCE_REPOSITORY, "Plan source repository must be the official upstream.")
    for key in ("sha", "range_base_sha", "fork_baseline_sha"):
        require(isinstance(source.get(key), str) and SHA_RE.fullmatch(source[key]), f"Source {key} must be a full SHA.")
    require(isinstance(downstream.get("base_sha"), str) and SHA_RE.fullmatch(downstream["base_sha"]), "Downstream base_sha must be a full SHA.")
    tag = source.get("tag")
    require(isinstance(tag, str) and tag, "Source release tag is required.")
    git(repo, "check-ref-format", f"refs/tags/{tag}")
    require(resolve_commit(repo, f"refs/tags/{tag}") == source["sha"], "Source tag moved or does not match frozen target SHA.")
    require(fork_baseline(repo) == source["fork_baseline_sha"], "Initial fork baseline cannot be changed.")
    require(is_ancestor(repo, source["fork_baseline_sha"], source["range_base_sha"])
            and is_ancestor(repo, source["range_base_sha"], source["sha"]), "Invalid upstream evaluation range.")
    require(is_ancestor(repo, source["fork_baseline_sha"], downstream["base_sha"]), "Downstream base must retain initial fork history.")
    if require_base:
        require(resolve_commit(repo, "HEAD") == downstream["base_sha"], "Downstream base changed; regenerate the assessment against the current base.")
    require(git_text(repo, "show", f"{downstream['base_sha']}:VERSION") == downstream.get("version"), "Pinned downstream VERSION does not match plan.")
    validate_version(downstream["version"], downstream.get("proposed_version", ""))
    if check_clean:
        checkout_clean(repo)
    require(not (repo / "docs/upstream-sync/history" / f"{plan['batch_id']}.json").exists(), "Batch has already been finalized; replay is forbidden.")
    items = plan.get("items")
    require(isinstance(items, list) and items, "Plan must include every source commit in the evaluated range.")
    expected = range_commits(repo, source["range_base_sha"], source["sha"])
    require([item.get("source_sha") for item in items if isinstance(item, dict)] == expected,
            "Source commit list/order changed or includes commits outside the evaluated range.")
    prior_decisions, follow_up = historical_evidence(history_records(repo), set(expected))
    require(plan.get("historical_follow_up") == follow_up, "Historical follow-up evidence changed; regenerate the assessment.")
    previous = {}
    for item in items:
        immutable = commit_info(repo, item["source_sha"])
        require(all(item.get(key) == value for key, value in immutable.items()), "Immutable commit evidence was modified: " + immutable["id"])
        require(item.get("analysis", {}).get("prior_decisions") == prior_decisions.get(item["source_sha"], []),
                "Historical decision evidence changed; regenerate the assessment.")
        decision = item.get("decision")
        require(decision in DECISIONS and decision != "pending", f"{item['id']}: choose an explicit decision.")
        require(isinstance(item.get("reason"), str) and item["reason"].strip(), f"{item['id']}: decision reason is required.")
        require(isinstance(item.get("feature"), str) and item["feature"].strip(), f"{item['id']}: feature/group is required.")
        require(isinstance(item.get("evidence"), list) and all(isinstance(entry, str) and entry.strip() for entry in item["evidence"]), "Evidence must be a list of nonempty strings.")
        require(decision != "covered" or item["evidence"], f"{item['id']}: covered requires code/test evidence.")
        dependencies = item.get("dependencies")
        require(isinstance(dependencies, list) and len(dependencies) == len(set(dependencies))
                and all(isinstance(entry, str) for entry in dependencies), "Dependencies must be distinct item IDs.")
        for dependency in dependencies:
            require(dependency in previous, f"{item['id']}: dependencies must refer to an earlier item.")
            if decision in ADOPTED or decision == "covered":
                require(previous[dependency] in ADOPTED | {"covered"}, f"{item['id']}: dependency {dependency} is not selected or covered.")
        scope = item.get("ai_scope")
        if scope is not None:
            require(decision in ADOPTED and isinstance(scope, dict) and set(scope) == {"paths", "instructions"},
                    "ai_scope must specify exact paths and instructions for a selected item.")
            paths = scope["paths"]
            require(isinstance(paths, list) and paths and all(isinstance(path, str) and path for path in paths)
                    and len(paths) == len(set(paths)), "AI scope paths must be a nonempty list of distinct exact file paths.")
            require(isinstance(scope["instructions"], str) and scope["instructions"].strip(),
                    "AI scope requires human-approved migration instructions.")
            for path in paths:
                require(not protected_path(path), "AI scope cannot include protected paths.")
                repository_path(repo, path)
        if decision in {"merge", "upgrade"}:
            require(len(item["parents"]) == 1, "Merge commits require a reviewed migration patch; no implicit mainline selection.")
            check_patch_safety(repo, item["parents"][0], item["source_sha"])
        if decision == "migrate":
            require(item.get("selected_scope") in {"reviewed_patch", "ai_adaptation"} and item["evidence"],
                    "migrate requires an explicit reviewed_patch or ai_adaptation scope and implementation evidence.")
            if item["selected_scope"] == "ai_adaptation":
                require(scope is not None and item.get("patch") is None,
                        "AI migration requires an approved ai_scope and cannot replace a reviewed patch.")
            else:
                require(scope is None, "Reviewed migration patches cannot also request AI adaptation.")
                migration_safety(repo, migration_patch(item, plan_root or repo))
        else:
            require(item.get("selected_scope") == "full_commit", "Non-migration items must retain selected_scope=full_commit.")
            require(item.get("patch") is None, "Only migrate may specify a patch.")
        previous[item["id"]] = decision
    return plan


def validate(args):
    plan = validate_plan(args.repo, args.plan, args.plan_root)
    print(json.dumps({"status": "valid", "batch_id": plan["batch_id"], "plan_sha256": plan_digest(plan)}))


def verify_candidate_mapping(repo, plan, mappings, head, plan_root):
    selected = [(item, mapping) for item, mapping in zip(plan["items"], mappings) if item["decision"] in ADOPTED]
    actual = range_commits(repo, plan["downstream"]["base_sha"], head)
    require([mapping["candidate_sha"] for _, mapping in selected] == actual,
            "Adopted mappings must exactly match the ordered candidate commits after the pinned base.")
    previous = plan["downstream"]["base_sha"]
    for item, mapping in selected:
        mapped = mapping["candidate_sha"]
        require(git_text(repo, "show", "-s", "--format=%P", mapped).split() == [previous],
                "Candidate commits must form a linear chain with one commit per adopted item.")
        content = (migration_patch(item, plan_root) if item["decision"] == "migrate" else
                   git(repo, "diff", "--binary", "--full-index", item["parents"][0], item["source_sha"]).stdout)
        with tempfile.TemporaryDirectory(prefix="upstream-sync-index-") as temp_dir:
            environment = dict(os.environ, GIT_INDEX_FILE=str(Path(temp_dir) / "index"))
            git(repo, "read-tree", previous, env=environment)
            options = ["apply", "--cached"]
            if item["decision"] in {"merge", "upgrade"}:
                options.append("--3way")
            git(repo, *options, "-", input_data=content, env=environment)
            expected_tree = git(repo, "write-tree", env=environment).stdout.decode().strip()
        require(expected_tree == git_text(repo, "rev-parse", f"{mapped}^{{tree}}"),
                "Candidate commit content does not match its selected source or reviewed migration patch.")
        previous = mapped


def apply(args):
    repo = Path(args.repo).resolve()
    plan = validate_plan(repo, args.plan, args.plan_root)
    require(not any(item.get("selected_scope") == "ai_adaptation" for item in plan["items"]),
            "Use upstream_sync_handoff.py finalize for manual AI adaptations; deterministic apply cannot generate code.")
    require(any(item["decision"] in ADOPTED for item in plan["items"]),
            "No items selected for adoption; keep the assessment without applying or advancing VERSION.")
    candidate = Path(args.candidate).resolve()
    require(not candidate.exists(), "Candidate path already exists; refusing to reset or reuse user data.")
    require(not candidate.is_relative_to(repo), "Candidate must be isolated outside the source checkout.")
    result_dir = Path(args.result_dir).resolve()
    require(not result_dir.exists(), "Result directory already exists; use a fresh execution path.")
    require(not result_dir.is_relative_to(repo), "Execution logs must be outside the clean source checkout.")
    result_dir.mkdir(parents=True)
    result = {"schema_version": 1, "batch_id": plan["batch_id"], "status": "applying",
              "base_sha": plan["downstream"]["base_sha"], "plan_sha256": plan_digest(plan),
              "candidate_path": str(candidate), "mappings": []}
    write_json(result_dir / "result.json", result)
    log = result_dir / "apply.log"
    try:
        git(repo, "worktree", "add", "--detach", str(candidate), result["base_sha"])
        for item in plan["items"]:
            if item["decision"] not in ADOPTED:
                result["mappings"].append({"item_id": item["id"], "source_sha": item["source_sha"],
                                           "decision": item["decision"], "candidate_sha": None})
                continue
            before = resolve_commit(candidate, "HEAD")
            with log.open("a", encoding="utf-8") as output:
                output.write(f"{item['id']} {item['decision']} {item['source_sha']}\n")
            if item["decision"] in {"merge", "upgrade"}:
                git(candidate, "cherry-pick", "-x", item["source_sha"])
            else:
                content = migration_patch(item, args.plan_root or repo)
                git(candidate, "apply", "--index", "--whitespace=error", "-", input_data=content)
                check_patch_safety(candidate, before, cached=True)
                git(candidate, "commit", "-m", f"Adapt upstream: {item['feature']}\n\nUpstream-source: {item['source_sha']}\nSync-batch: {plan['batch_id']}")
            after = resolve_commit(candidate, "HEAD")
            check_patch_safety(candidate, before, after)
            result["mappings"].append({"item_id": item["id"], "source_sha": item["source_sha"],
                                       "decision": item["decision"], "candidate_sha": after})
        checkout_clean(candidate)
        result.update({"status": "applied", "candidate_head": resolve_commit(candidate, "HEAD")})
    except (SyncError, OSError) as error:
        result.update({"status": "failed", "error": str(error)})
        with log.open("a", encoding="utf-8") as output:
            output.write(str(error) + "\n")
        write_json(result_dir / "result.json", result)
        raise SyncError(f"Application failed; isolated candidate and logs retained at {result_dir}: {error}") from error
    write_json(result_dir / "result.json", result)
    print(json.dumps(result))


def validate_execution(repo, plan, result, verification, plan_root):
    require(any(item["decision"] in ADOPTED for item in plan["items"]),
            "Execution evidence requires at least one adopted item; assessments alone cannot advance VERSION.")
    require(result.get("plan_sha256") == plan_digest(plan), "Plan changed after execution.")
    require(result.get("batch_id") == plan.get("batch_id") and result.get("base_sha") == plan.get("downstream", {}).get("base_sha"), "Execution result does not match plan.")
    head = result.get("candidate_head")
    require(isinstance(head, str) and SHA_RE.fullmatch(head) and is_ancestor(repo, result["base_sha"], head), "Invalid candidate application HEAD.")
    require(verification.get("status") == "passed" and verification.get("validated_head") == head, "Verification must pass against the exact applied candidate HEAD.")
    checks = verification.get("checks")
    require(isinstance(checks, list) and len(checks) == len(REQUIRED_CHECKS)
            and {entry.get("name") for entry in checks if isinstance(entry, dict)} == REQUIRED_CHECKS
            and all(isinstance(entry, dict) and entry.get("status") == "passed" for entry in checks),
            "All fixed frontend/backend/independent-relaykit verification checks must pass.")
    require(result.get("mappings") == [{"item_id": item["id"], "source_sha": item["source_sha"],
                                        "decision": item["decision"], "candidate_sha": mapping.get("candidate_sha")}
                                       for item, mapping in zip(plan["items"], result.get("mappings", []))]
            and len(result.get("mappings", [])) == len(plan["items"]), "Result mappings must cover every reviewed decision.")
    for item, mapping in zip(plan["items"], result["mappings"]):
        mapped = mapping["candidate_sha"]
        if item["decision"] in ADOPTED:
            require(isinstance(mapped, str) and SHA_RE.fullmatch(mapped) and is_ancestor(repo, mapped, head), "Adopted item lacks a candidate commit mapping.")
        else:
            require(mapped is None, "Skipped/deferred/covered items cannot have candidate commit mappings.")
    verify_candidate_mapping(repo, plan, result["mappings"], head, plan_root)
    check_patch_safety(repo, result["base_sha"], head)


def metadata_files(repo, plan, result, verification, plan_root):
    """Render exact expected metadata from immutable Git objects, without checkout."""
    head = result["candidate_head"]
    record = {"schema_version": 1, "batch_id": plan["batch_id"], "state": "validated_candidate",
              "integration": "selective", "source": plan["source"], "downstream": plan["downstream"],
              "candidate_head": head, "plan_sha256": plan_digest(plan), "verification": verification,
              "mappings": result["mappings"],
              "decisions": [{key: item[key] for key in ("id", "source_sha", "feature", "decision", "selected_scope", "reason", "dependencies", "evidence", "patch")} for item in plan["items"]]}
    directory = f"docs/upstream-sync/plans/{plan['batch_id']}"
    record_path = f"docs/upstream-sync/history/{plan['batch_id']}.json"
    for path in (directory, record_path):
        require(git(repo, "cat-file", "-e", f"{head}:{path}", check=False).returncode != 0,
                "Plan or history destination already exists; refusing to overwrite existing records.")
    files = {f"{directory}/plan.json": canonical(plan),
             f"{directory}/report.md": render_report(plan).encode("utf-8"),
             record_path: canonical(record),
             "VERSION": (plan["downstream"]["proposed_version"] + "\n").encode("utf-8")}
    for item in plan["items"]:
        if item["decision"] != "migrate":
            continue
        destination = item["patch"]["path"]
        content = migration_patch(item, plan_root)
        existing = git(repo, "show", f"{head}:{destination}", check=False)
        require(existing.returncode != 0 or existing.stdout == content, "Cannot replace an existing migration patch.")
        require(destination not in files or files[destination] == content, "Migration patches reuse a path with different content.")
        files[destination] = content
    upstream = git(repo, "show", f"{head}:UPSTREAM.md").stdout
    appendix = f"\n## Selective synchronization batch `{plan['batch_id']}`\n\n"
    appendix += f"Evaluated [{plan['source']['tag']}]({SOURCE_REPOSITORY}/releases/tag/{plan['source']['tag']}) at `{plan['source']['sha']}`; range `{plan['source']['range_base_sha']}..{plan['source']['sha']}`. Initial fork baseline remains `{plan['source']['fork_baseline_sha']}`. This is selective integration, not a full baseline upgrade.\n\n"
    appendix += f"Downstream version: `{plan['downstream']['version']}` → `{plan['downstream']['proposed_version']}`. All seven required checks passed on candidate `{head}`. These records describe a validated PR candidate; merge and publication remain separate actions.\n\n"
    appendix += f"[Assessment and decisions]({directory}/report.md) · [Machine-readable history]({record_path})\n"
    for item, mapping in zip(plan["items"], result["mappings"]):
        candidate_text = f" → `{mapping['candidate_sha']}`" if mapping["candidate_sha"] else ""
        appendix += f"\n- `{item['decision']}` [{item['source_sha']}]({SOURCE_REPOSITORY}/commit/{item['source_sha']}){candidate_text}: {item['reason']}\n"
    files["UPSTREAM.md"] = upstream + appendix.encode("utf-8")
    return files


def verify_candidate(args):
    """Verify imported candidate objects before a separate trusted publisher pushes."""
    repo = Path(args.repo).resolve()
    plan = validate_plan(repo, args.plan, args.plan_root)
    result, verification = read_json(args.result), read_json(args.verification)
    require(result.get("status") == "finalized", "Publisher requires a finalized candidate.")
    validate_execution(repo, plan, result, verification, args.plan_root or repo)
    metadata_commit = result.get("metadata_commit")
    require(isinstance(metadata_commit, str) and SHA_RE.fullmatch(metadata_commit), "Invalid metadata commit.")
    require(git_text(repo, "show", "-s", "--format=%P", metadata_commit).split() == [result["candidate_head"]],
            "Metadata commit must directly follow the validated runtime candidate.")
    files = metadata_files(repo, plan, result, verification, args.plan_root or repo)
    with tempfile.TemporaryDirectory(prefix="upstream-sync-metadata-") as temp_dir:
        environment = dict(os.environ, GIT_INDEX_FILE=str(Path(temp_dir) / "index"))
        git(repo, "read-tree", result["candidate_head"], env=environment)
        for path, content in files.items():
            blob = git(repo, "hash-object", "-w", "--stdin", input_data=content).stdout.decode().strip()
            git(repo, "update-index", "--add", "--cacheinfo", "100644", blob, path, env=environment)
        tree = git(repo, "write-tree", env=environment).stdout.decode().strip()
    require(tree == git_text(repo, "rev-parse", f"{metadata_commit}^{{tree}}"),
            "Published metadata tree differs from the exact trusted plan/source/verification records.")
    print(json.dumps({"status": "verified", "metadata_commit": metadata_commit, "batch_id": plan["batch_id"]}))


def finalize(args):
    repo = Path(args.repo).resolve()
    result, plan = read_json(args.result), read_json(args.plan)
    require(Path(result.get("candidate_path", "")).resolve() == repo, "Finalization must use the isolated candidate recorded by apply.")
    record_path = repo / "docs/upstream-sync/history" / f"{plan.get('batch_id')}.json"
    if result.get("status") == "finalized":
        require(record_path.is_file() and read_json(record_path).get("plan_sha256") == plan_digest(plan)
                and resolve_commit(repo, "HEAD") == result.get("metadata_commit"), "Finalized result does not match candidate history.")
        checkout_clean(repo)
        print(json.dumps(result))
        return
    require(result.get("status") == "applied", "Only a successfully applied candidate may be finalized.")
    plan = validate_plan(repo, args.plan, args.plan_root, require_base=False)
    require(resolve_commit(repo, "HEAD") == result.get("candidate_head"), "Candidate changed after application.")
    verification = read_json(args.verification)
    validate_execution(repo, plan, result, verification, args.plan_root or repo)
    files = metadata_files(repo, plan, result, verification, args.plan_root or repo)
    destinations = {path: repository_path(repo, path) for path in files}
    for path, content in files.items():
        destination = destinations[path]
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(content)
    git(repo, "add", "--", "VERSION", "UPSTREAM.md", "docs/upstream-sync")
    git(repo, "commit", "-m", f"docs: record selective upstream sync {plan['batch_id']}")
    result.update({"status": "finalized", "metadata_commit": resolve_commit(repo, "HEAD"),
                   "record_path": str(record_path.relative_to(repo))})
    write_json(args.result, result)
    print(json.dumps(result))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    analysis = commands.add_parser("analyze")
    analysis.add_argument("--repo", default=".")
    analysis.add_argument("--source-repo", default=SOURCE_REPOSITORY)
    analysis.add_argument("--target", required=True)
    analysis.add_argument("--range-base", default="8f6961c675932f406260ff0c218bc2aa0603e9b2")
    analysis.add_argument("--batch", required=True)
    analysis.add_argument("--version", required=True)
    analysis.add_argument("--output-dir", required=True)
    for name in ("validate", "apply", "finalize", "verify-candidate"):
        command = commands.add_parser(name)
        command.add_argument("--repo", default=".")
        command.add_argument("--plan", required=True)
        command.add_argument("--plan-root")
        if name == "apply":
            command.add_argument("--candidate", required=True)
            command.add_argument("--result-dir", required=True)
        if name in {"finalize", "verify-candidate"}:
            command.add_argument("--result", required=True)
            command.add_argument("--verification", required=True)
    args = parser.parse_args()
    try:
        {"analyze": analyze, "validate": validate, "apply": apply, "finalize": finalize,
         "verify-candidate": verify_candidate}[args.command](args)
    except (SyncError, OSError, KeyError, TypeError, ValueError) as error:
        print(f"upstream-sync: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
