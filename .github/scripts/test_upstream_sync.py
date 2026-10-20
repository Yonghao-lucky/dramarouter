"""Exercise upstream sync plans against isolated, deterministic Git histories."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("upstream_sync.py").resolve()
SOURCE = "https://github.com/QuantumNous/new-api"


class UpstreamSyncIntegrationTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="upstream-sync-test-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.repo = self.root / "repository"
        self.repo.mkdir()
        self.env = os.environ.copy()
        self.env.update({
            "GIT_AUTHOR_DATE": "2026-10-16T00:00:00+00:00",
            "GIT_COMMITTER_DATE": "2026-10-16T00:00:00+00:00",
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_TERMINAL_PROMPT": "0",
        })
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Fixture Maintainer")
        self.git("config", "user.email", "fixture@example.invalid")
        self.git("config", "commit.gpgsign", "false")
        self.write("provider.txt", "existing provider behavior\n")
        self.write("independent.txt", "existing independent behavior\n")
        self.write("README.md", "# new-api\nMaintained by QuantumNous\n")
        self.write("LICENSE", "Fixture license for new-api by QuantumNous\n")
        self.commit("Initial upstream behavior")
        self.baseline = self.git("rev-parse", "HEAD").strip()
        self.git("tag", "v1.0.0")
        self.git("switch", "-c", "upstream-fixture")
        self.write("provider.txt", "upstream provider fix\n")
        self.commit("Fix provider request handling")
        self.first = self.git("rev-parse", "HEAD").strip()
        self.write("independent.txt", "upstream independent improvement\n")
        self.commit("Improve independent behavior")
        self.second = self.git("rev-parse", "HEAD").strip()
        self.git("tag", "v1.1.0")
        self.target = self.second
        self.git("switch", "main")
        self.write("VERSION", "v1.0.0-rc.26-novamaas.2\n")
        self.write("UPSTREAM.md", (
            "# Upstream provenance / 上游来源\n\n"
            "- Project / 项目：New API\n"
            f"- Repository / 仓库：{SOURCE}\n"
            "- Baseline release / 初始版本：`v1.0.0-rc.26`\n"
            f"- Commit / 原始提交：`{self.baseline}`\n\n"
            "## 已选择同步的上游改动\n"
        ))
        self.write("downstream.txt", "local business behavior\n")
        self.commit("Record downstream fork provenance")
        self.base = self.git("rev-parse", "HEAD").strip()
        self.plan_dir = self.root / "plans"
        self.candidate = self.root / "candidate"
        self.results = self.root / "results"

    def git(self, *args, repo=None, check=True):
        result = subprocess.run(
            ["git", "-C", str(repo or self.repo), *args],
            env=self.env, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, check=False,
        )
        if check and result.returncode:
            self.fail(f"git {' '.join(args)} failed:\n{result.stderr}")
        return result.stdout if check else result

    def write(self, name, content, root=None):
        destination = (root or self.repo) / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(content, encoding="utf-8")
        return destination

    def commit(self, message, repo=None):
        self.git("add", "--all", repo=repo)
        self.git("commit", "-m", message, repo=repo)

    def cli(self, command, *args, success=True):
        result = subprocess.run(
            [sys.executable, str(SCRIPT), command, *map(str, args)],
            env=self.env, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, check=False,
        )
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def analyze(self, range_base=None):
        self.cli(
            "analyze", "--repo", self.repo, "--source-repo", SOURCE,
            "--target", "v1.1.0", "--range-base", range_base or self.baseline,
            "--batch", "fixture-20261016", "--version",
            "v1.0.0-rc.26-novamaas.3", "--output-dir", self.plan_dir,
        )
        plans = list(self.plan_dir.glob("*.json"))
        self.assertEqual(len(plans), 1, f"Expected one generated plan: {plans}")
        self.plan_path = plans[0]
        self.plan = json.loads(self.plan_path.read_text(encoding="utf-8"))
        return self.plan

    def save_plan(self):
        self.plan_path.write_text(
            json.dumps(self.plan, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )

    def reviewed_plan(self):
        self.analyze()
        for item in self.plan["items"]:
            item["decision"] = "skip"
            item["reason"] = "Not needed by this downstream release."
        self.save_plan()

    def apply(self, success=True):
        return self.cli(
            "apply", "--repo", self.repo, "--plan", self.plan_path,
            "--plan-root", self.plan_dir, "--candidate", self.candidate,
            "--result-dir", self.results, success=success,
        )

    def result_path(self):
        paths = list(self.results.glob("*.json"))
        self.assertEqual(len(paths), 1, f"Expected one execution result: {paths}")
        return paths[0]

    def verification(self, **changes):
        evidence = {
            "status": "passed",
            "validated_head": self.git("rev-parse", "HEAD", repo=self.candidate).strip(),
            "checks": [
                {"name": name, "status": "passed"}
                for name in (
                    "frontend-types", "frontend-tests", "frontend-build",
                    "backend-tests", "backend-build", "relaykit-tests",
                    "relaykit-independent-build",
                )
            ],
        }
        evidence.update(changes)
        path = self.root / "verification.json"
        path.write_text(json.dumps(evidence), encoding="utf-8")
        return path

    def finalize(self, verification, success=True):
        return self.cli(
            "finalize", "--repo", self.candidate, "--plan", self.plan_path,
            "--result", self.result_path(), "--verification", verification,
            success=success,
        )

    def verify_candidate(self, verification, success=True):
        return self.cli(
            "verify-candidate", "--repo", self.repo, "--plan", self.plan_path,
            "--plan-root", self.plan_dir, "--result", self.result_path(),
            "--verification", verification, success=success,
        )

    def repository_state(self):
        return (
            self.git("rev-parse", "HEAD"),
            self.git("status", "--porcelain=v1", "--untracked-files=all"),
            self.git("diff", "--binary", "HEAD"),
        )

    def test_selective_merge_finalizes_only_verified_adopted_sources(self):
        self.reviewed_plan()
        items = {item["source_sha"]: item for item in self.plan["items"]}
        items[self.first]["decision"] = "merge"
        items[self.first]["reason"] = "Fix required for downstream provider requests."
        items[self.second]["decision"] = "defer"
        items[self.second]["reason"] = "Independent improvement requires separate review."
        self.save_plan()
        before = self.repository_state()

        self.cli("validate", "--repo", self.repo, "--plan", self.plan_path)
        self.apply()

        self.assertEqual(self.repository_state(), before)
        self.assertEqual((self.candidate / "provider.txt").read_text(), "upstream provider fix\n")
        self.assertEqual((self.candidate / "independent.txt").read_text(), "existing independent behavior\n")
        self.assertEqual((self.candidate / "downstream.txt").read_text(), "local business behavior\n")
        result = json.loads(self.result_path().read_text())
        self.assertEqual(result["status"], "applied")
        self.assertEqual(
            [mapping["source_sha"] for mapping in result["mappings"] if mapping["candidate_sha"]],
            [self.first],
        )
        self.assertEqual((self.candidate / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.2")

        self.finalize(self.verification())

        self.assertEqual((self.candidate / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.3")
        provenance = (self.candidate / "UPSTREAM.md").read_text()
        self.assertIn(f"- Commit / 原始提交：`{self.baseline}`", provenance)
        self.assertIn("v1.1.0", provenance)
        history_path = self.candidate / "docs/upstream-sync/history/fixture-20261016.json"
        history = json.loads(history_path.read_text())
        self.assertEqual(history["source"]["sha"], self.target)
        self.assertEqual(history["integration"], "selective")
        self.assertEqual(
            [mapping["source_sha"] for mapping in history["mappings"] if mapping["candidate_sha"]],
            [self.first],
        )
        recorded = self.candidate / "docs/upstream-sync/plans/fixture-20261016/plan.json"
        self.assertEqual(json.loads(recorded.read_text()), self.plan)
        self.assertEqual(self.repository_state(), before)
        self.assertEqual(self.git("branch", "--show-current").strip(), "main")

    def test_unreviewed_or_incomplete_dependencies_cannot_create_candidate(self):
        self.reviewed_plan()
        valid = json.loads(json.dumps(self.plan))
        first_id = valid["items"][0]["id"]
        cases = {
            "pending decision": {"decision": "pending"},
            "missing reason": {"decision": "merge", "reason": ""},
            "missing dependency": {"decision": "merge", "dependencies": ["missing-item"]},
            "skipped dependency": {"decision": "merge", "dependencies": [first_id]},
            "unproven coverage": {"decision": "covered", "evidence": []},
        }
        before = self.repository_state()
        for label, changes in cases.items():
            with self.subTest(label=label):
                self.plan = json.loads(json.dumps(valid))
                self.plan["items"][1].update(changes)
                self.save_plan()
                self.apply(success=False)
                self.assertEqual(self.repository_state(), before)
                self.assertFalse(self.candidate.exists())

    def test_no_adopted_items_cannot_create_candidate_or_bump_version(self):
        self.reviewed_plan()
        self.plan["items"][0].update({
            "decision": "covered",
            "reason": "Existing local implementation already satisfies this behavior.",
            "evidence": ["provider.txt and its explicit local review cover the requested behavior."],
        })
        self.plan["items"][1].update({
            "decision": "defer",
            "reason": "Require independent compatibility review before adoption.",
        })
        self.save_plan()
        before = self.repository_state()

        self.apply(success=False)

        self.assertEqual(self.repository_state(), before)
        self.assertFalse(self.candidate.exists())
        self.assertFalse(self.results.exists())
        self.assertEqual((self.repo / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.2")

    def test_pinned_source_and_base_cannot_be_replaced_by_plan_edits(self):
        self.reviewed_plan()
        valid = json.loads(json.dumps(self.plan))
        cases = (
            ("source", "repository", "https://example.invalid/untrusted/repository"),
            ("source", "sha", self.first),
            ("source", "range_base_sha", self.first),
            ("source", "fork_baseline_sha", self.first),
            ("downstream", "base_sha", self.baseline),
            ("downstream", "version", "v1.0.0-rc.26-novamaas.1"),
            ("downstream", "proposed_version", "v1.0.0-rc.26-novamaas.2"),
        )
        before = self.repository_state()
        for section, field, value in cases:
            with self.subTest(field=f"{section}.{field}"):
                self.plan = json.loads(json.dumps(valid))
                self.plan[section][field] = value
                self.save_plan()
                self.apply(success=False)
                self.assertEqual(self.repository_state(), before)
                self.assertFalse(self.candidate.exists())

    def test_retargeted_tag_and_new_downstream_commit_require_reassessment(self):
        self.reviewed_plan()
        self.git("tag", "-f", "v1.1.0", self.first)
        before = self.repository_state()
        self.apply(success=False)
        self.assertEqual(self.repository_state(), before)
        self.assertFalse(self.candidate.exists())
        self.git("tag", "-f", "v1.1.0", self.target)
        self.write("downstream.txt", "new downstream business decision\n")
        self.commit("Advance downstream after assessment")
        before = self.repository_state()
        self.apply(success=False)
        self.assertEqual(self.repository_state(), before)
        self.assertFalse(self.candidate.exists())

    def test_same_plan_cannot_reapply_on_its_candidate(self):
        self.reviewed_plan()
        self.plan["items"][0]["decision"] = "merge"
        self.save_plan()
        self.apply()
        first_head = self.git("rev-parse", "HEAD", repo=self.candidate)
        self.cli(
            "apply", "--repo", self.candidate, "--plan", self.plan_path,
            "--candidate", self.root / "repeated", "--result-dir", self.root / "repeated-result",
            success=False,
        )
        self.assertEqual(self.git("rev-parse", "HEAD", repo=self.candidate), first_head)
        self.assertFalse((self.root / "repeated").exists())

    def migration_patch(self, path="docs/upstream-sync/patches/provider.patch", patch=None):
        if patch is None:
            patch = (
                "diff --git a/provider.txt b/provider.txt\n"
                "--- a/provider.txt\n"
                "+++ b/provider.txt\n"
                "@@ -1 +1 @@\n"
                "-existing provider behavior\n"
                "+adapted provider fix preserving downstream semantics\n"
            )
        destination = self.write(path, patch, root=self.plan_dir)
        return {"path": path, "sha256": hashlib.sha256(destination.read_bytes()).hexdigest()}

    def test_migration_applies_reviewed_patch_instead_of_full_upstream_commit(self):
        self.reviewed_plan()
        item = self.plan["items"][0]
        item["decision"] = "migrate"
        item["reason"] = "Preserve downstream semantics while adopting upstream behavior."
        item["selected_scope"] = "reviewed_patch"
        item["evidence"] = ["Adopt only provider behavior; preserve downstream semantics."]
        item["patch"] = self.migration_patch()
        self.save_plan()
        self.apply()
        self.assertEqual(
            (self.candidate / "provider.txt").read_text(),
            "adapted provider fix preserving downstream semantics\n",
        )
        mapping = json.loads(self.result_path().read_text())["mappings"][0]
        self.assertEqual(mapping["decision"], "migrate")
        self.assertEqual(mapping["source_sha"], self.first)
        self.assertEqual((self.repo / "provider.txt").read_text(), "existing provider behavior\n")
        self.cli(
            "finalize", "--repo", self.candidate, "--plan", self.plan_path,
            "--plan-root", self.plan_dir, "--result", self.result_path(),
            "--verification", self.verification(),
        )
        archived = self.candidate / item["patch"]["path"]
        self.assertEqual(hashlib.sha256(archived.read_bytes()).hexdigest(), item["patch"]["sha256"])
        history = json.loads((self.candidate / "docs/upstream-sync/history/fixture-20261016.json").read_text())
        self.assertEqual(history["integration"], "selective")
        self.assertEqual(history["decisions"][0]["selected_scope"], "reviewed_patch")

    def test_unreviewed_patch_bytes_and_escaping_patch_paths_are_rejected(self):
        self.reviewed_plan()
        item = self.plan["items"][0]
        item["decision"] = "migrate"
        item["selected_scope"] = "reviewed_patch"
        item["evidence"] = ["Reviewed provider-only adaptation."]
        item["patch"] = self.migration_patch()
        approved = json.loads(json.dumps(self.plan))
        before = self.repository_state()
        cases = (
            {"sha256": "0" * 64},
            {"path": "../provider.patch"},
            {"path": str(self.root / "provider.patch")},
        )
        for changes in cases:
            with self.subTest(changes=changes):
                self.plan = json.loads(json.dumps(approved))
                self.plan["items"][0]["patch"].update(changes)
                self.save_plan()
                self.apply(success=False)
                self.assertEqual(self.repository_state(), before)
                self.assertFalse(self.candidate.exists())

    def test_patch_symlink_cannot_read_outside_reviewed_plan_root(self):
        self.reviewed_plan()
        item = self.plan["items"][0]
        item["decision"] = "migrate"
        item["selected_scope"] = "reviewed_patch"
        item["evidence"] = ["Reviewed provider-only adaptation."]
        item["patch"] = self.migration_patch()
        destination = self.plan_dir / item["patch"]["path"]
        outside = self.root / "unreviewed.patch"
        destination.rename(outside)
        destination.symlink_to(outside)
        self.save_plan()
        before = self.repository_state()
        self.apply(success=False)
        self.assertEqual(self.repository_state(), before)
        self.assertFalse(self.candidate.exists())

    def test_patches_cannot_modify_ci_governance_or_upstream_attribution(self):
        self.reviewed_plan()
        approved = json.loads(json.dumps(self.plan))
        patches = {
            "workflow": (
                "diff --git a/.github/workflows/unsafe.yml b/.github/workflows/unsafe.yml\n"
                "new file mode 100644\n"
                "--- /dev/null\n"
                "+++ b/.github/workflows/unsafe.yml\n"
                "@@ -0,0 +1 @@\n"
                "+name: unreviewed workflow\n"
            ),
            "attribution": (
                "diff --git a/README.md b/README.md\n"
                "--- a/README.md\n"
                "+++ b/README.md\n"
                "@@ -1,2 +1,2 @@\n"
                " # new-api\n"
                "-Maintained by QuantumNous\n"
                "+Maintained downstream\n"
            ),
            "traversal": (
                "diff --git a/../escaped.txt b/../escaped.txt\n"
                "new file mode 100644\n"
                "--- /dev/null\n"
                "+++ b/../escaped.txt\n"
                "@@ -0,0 +1 @@\n"
                "+outside candidate\n"
            ),
        }
        before = self.repository_state()
        for label, patch in patches.items():
            with self.subTest(label=label):
                self.plan = json.loads(json.dumps(approved))
                self.plan["items"][0]["decision"] = "migrate"
                self.plan["items"][0]["selected_scope"] = "reviewed_patch"
                self.plan["items"][0]["evidence"] = ["Review this isolated adaptation."]
                self.plan["items"][0]["patch"] = self.migration_patch(patch=patch)
                self.save_plan()
                self.candidate = self.root / ("candidate-" + label)
                self.results = self.root / ("results-" + label)
                self.apply(success=False)
                self.assertEqual(self.repository_state(), before)
                self.assertFalse((self.root / "escaped.txt").exists())

    def test_forged_file_list_cannot_hide_protected_upstream_changes(self):
        self.git("switch", "upstream-fixture")
        self.write(".github/workflows/unreviewed.yml", "name: unsafe source workflow\n")
        self.commit("Change upstream workflow")
        protected_sha = self.git("rev-parse", "HEAD").strip()
        self.git("tag", "-f", "v1.1.0")
        self.git("switch", "main")
        self.reviewed_plan()
        item = next(item for item in self.plan["items"] if item["source_sha"] == protected_sha)
        item["decision"] = "merge"
        item["files"] = ["provider.txt"]
        self.save_plan()
        before = self.repository_state()
        self.apply(success=False)
        self.assertEqual(self.repository_state(), before)
        self.assertFalse(self.candidate.exists())

    def test_incomplete_or_stale_verification_cannot_write_release_records(self):
        self.reviewed_plan()
        self.plan["items"][0]["decision"] = "merge"
        self.save_plan()
        self.apply()
        original_version = (self.candidate / "VERSION").read_text()
        original_provenance = (self.candidate / "UPSTREAM.md").read_text()
        original_head = self.git("rev-parse", "HEAD", repo=self.candidate)
        cases = (
            {"status": "failed"},
            {"validated_head": self.baseline},
            {"checks": [{"name": "backend-tests", "status": "passed"}]},
        )
        for changes in cases:
            with self.subTest(changes=changes):
                self.finalize(self.verification(**changes), success=False)
                self.assertEqual((self.candidate / "VERSION").read_text(), original_version)
                self.assertEqual((self.candidate / "UPSTREAM.md").read_text(), original_provenance)
                self.assertEqual(self.git("rev-parse", "HEAD", repo=self.candidate), original_head)
                self.assertFalse((self.candidate / "docs/upstream-sync/history").exists())

    def test_result_cannot_claim_an_ancestor_as_an_adopted_commit(self):
        self.reviewed_plan()
        self.plan["items"][0]["decision"] = "merge"
        self.save_plan()
        self.apply()
        result_path = self.result_path()
        result = json.loads(result_path.read_text())
        result["mappings"][0]["candidate_sha"] = self.base
        result_path.write_text(json.dumps(result), encoding="utf-8")
        head = self.git("rev-parse", "HEAD", repo=self.candidate)

        self.finalize(self.verification(), success=False)

        self.assertEqual(self.git("rev-parse", "HEAD", repo=self.candidate), head)
        self.assertFalse((self.candidate / "docs/upstream-sync/history").exists())
        self.assertEqual((self.candidate / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.2")

    def test_existing_patch_collision_does_not_partially_write_release_records(self):
        patch_path = "docs/upstream-sync/patches/provider.patch"
        self.write(patch_path, "Existing separately reviewed adaptation\n")
        self.commit("Preserve existing downstream adaptation")
        self.reviewed_plan()
        item = self.plan["items"][0]
        item.update({
            "decision": "migrate", "selected_scope": "reviewed_patch",
            "evidence": ["Adapt only provider behavior."],
            "patch": self.migration_patch(path=patch_path),
        })
        self.save_plan()
        self.apply()
        original_provenance = (self.candidate / "UPSTREAM.md").read_text()
        head = self.git("rev-parse", "HEAD", repo=self.candidate)

        self.cli(
            "finalize", "--repo", self.candidate, "--plan", self.plan_path,
            "--plan-root", self.plan_dir, "--result", self.result_path(),
            "--verification", self.verification(), success=False,
        )

        self.assertEqual(self.git("rev-parse", "HEAD", repo=self.candidate), head)
        self.assertEqual(self.git("status", "--porcelain=v1", repo=self.candidate), "")
        self.assertEqual((self.candidate / "UPSTREAM.md").read_text(), original_provenance)
        self.assertEqual((self.candidate / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.2")
        self.assertFalse((self.candidate / "docs/upstream-sync/history").exists())
        self.assertFalse((self.candidate / "docs/upstream-sync/plans/fixture-20261016").exists())

    def test_trusted_verifier_accepts_exact_metadata_and_rejects_attribution_tampering(self):
        self.reviewed_plan()
        self.plan["items"][0]["decision"] = "merge"
        self.save_plan()
        self.apply()
        evidence = self.verification()
        self.finalize(evidence)
        before = self.repository_state()

        self.verify_candidate(evidence)
        self.assertEqual(self.repository_state(), before)

        provenance = self.candidate / "UPSTREAM.md"
        provenance.write_text(provenance.read_text().replace("New API", "Unreviewed replacement"))
        self.git("add", "UPSTREAM.md", repo=self.candidate)
        self.git("commit", "--amend", "--no-edit", repo=self.candidate)
        result_path = self.result_path()
        result = json.loads(result_path.read_text())
        result["metadata_commit"] = self.git("rev-parse", "HEAD", repo=self.candidate).strip()
        result_path.write_text(json.dumps(result), encoding="utf-8")

        self.verify_candidate(evidence, success=False)
        self.assertEqual(self.repository_state(), before)

    def test_forged_runtime_commit_cannot_be_recorded_as_selected_source(self):
        self.reviewed_plan()
        self.plan["items"][0]["decision"] = "merge"
        self.save_plan()
        self.apply()
        self.write("provider.txt", "unreviewed replacement runtime behavior\n", root=self.candidate)
        self.git("add", "provider.txt", repo=self.candidate)
        self.git("commit", "--amend", "--no-edit", repo=self.candidate)
        forged_head = self.git("rev-parse", "HEAD", repo=self.candidate).strip()
        result_path = self.result_path()
        result = json.loads(result_path.read_text())
        result["candidate_head"] = forged_head
        result["mappings"][0]["candidate_sha"] = forged_head
        result_path.write_text(json.dumps(result), encoding="utf-8")

        self.finalize(self.verification(), success=False)

        self.assertEqual(self.git("rev-parse", "HEAD", repo=self.candidate).strip(), forged_head)
        self.assertFalse((self.candidate / "docs/upstream-sync/history").exists())
        self.assertEqual((self.candidate / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.2")

    def test_narrower_assessment_retains_unresolved_history_until_later_coverage(self):
        previous = {
            "schema_version": 1, "batch_id": "z-earlier",
            "downstream": {"proposed_version": "v1.0.0-rc.26-novamaas.1"},
            "decisions": [
                {
                    "source_sha": self.first, "decision": "defer", "selected_scope": "full_commit",
                    "feature": "Provider compatibility", "reason": "Pending downstream protocol review.",
                },
                {
                    "source_sha": self.second, "decision": "migrate", "selected_scope": "reviewed_patch",
                    "feature": "Independent improvement", "reason": "Only a reviewed subset was adopted.",
                },
            ],
        }
        self.write("docs/upstream-sync/history/z-earlier.json", json.dumps(previous))
        self.commit("Record earlier selective assessment")

        self.analyze(range_base=self.first)

        self.assertEqual([item["source_sha"] for item in self.plan["items"]], [self.second])
        self.assertEqual(self.plan["items"][0]["decision"], "pending")
        self.assertEqual(self.plan["historical_follow_up"], [{
            "batch_id": "z-earlier", "source_sha": self.first, "decision": "defer",
            "selected_scope": "full_commit", "feature": "Provider compatibility",
            "reason": "Pending downstream protocol review.",
        }])
        self.assertEqual(self.plan["items"][0]["analysis"]["prior_decisions"], [{
            "batch_id": "z-earlier", "decision": "migrate", "selected_scope": "reviewed_patch",
            "reason": "Only a reviewed subset was adopted.",
        }])
        report = (self.plan_dir / "report.md").read_text()
        self.assertIn(self.first, report)
        self.assertIn("Pending downstream protocol review.", report)

        newer = {
            "schema_version": 1, "batch_id": "a-later",
            "downstream": {"proposed_version": "v1.0.0-rc.26-novamaas.2"},
            "decisions": [{
                "source_sha": self.first, "decision": "covered", "selected_scope": "full_commit",
                "feature": "Provider compatibility", "reason": "Equivalent downstream behavior now verified.",
            }],
        }
        self.write("docs/upstream-sync/history/a-later.json", json.dumps(newer))
        self.commit("Record later verified equivalent coverage")
        self.plan_dir = self.root / "later-plans"

        self.analyze(range_base=self.first)

        self.assertEqual(self.plan["historical_follow_up"], [])
        self.assertEqual(self.plan["items"][0]["decision"], "pending")


if __name__ == "__main__":
    unittest.main()
