"""Manual AI handoff stays in one PR and binds approval to exact source changes."""

import copy
import json
from pathlib import Path
import subprocess
import sys
import unittest

import test_upstream_sync as fixtures
import upstream_sync as engine


SCRIPT = Path(__file__).with_name("upstream_sync_handoff.py").resolve()


class UpstreamSyncHandoffTests(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.UpstreamSyncIntegrationTests()
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        f = self.fixture
        f.analyze()
        self.relative_plan = f"docs/upstream-sync/plans/{f.plan['batch_id']}/plan.json"
        self.history_path = f.repo / f"docs/upstream-sync/history/{f.plan['batch_id']}.json"
        f.git("switch", "-c", "codex/upstream-plan-fixture")
        self.save_plan()
        f.commit("Open draft upstream synchronization plan")
        self.planning_head = self.head()

    def head(self):
        return self.fixture.git("rev-parse", "HEAD").strip()

    def save_plan(self):
        f = self.fixture
        f.write(self.relative_plan, json.dumps(f.plan, ensure_ascii=False, indent=2) + "\n")
        f.write(str(Path(self.relative_plan).with_name("report.md")), engine.render_report(f.plan))

    def approve_plan(self, decision="merge", scope=None):
        f = self.fixture
        for item in f.plan["items"]:
            item.update({"decision": "skip", "reason": "Not selected for this downstream release."})
        first = f.plan["items"][0]
        first.update({
            "decision": decision,
            "reason": "Adopt provider behavior while preserving the downstream request contract.",
            "evidence": ["provider.txt contains the relevant downstream implementation."],
            "ai_scope": {
                "paths": scope or ["provider.txt"],
                "instructions": "Preserve the downstream request contract while adopting the selected provider behavior.",
            },
        })
        if decision == "migrate":
            first["selected_scope"] = "ai_adaptation"
        self.save_plan()
        f.commit("Maintainer confirms selected behavior and editable paths")
        self.approval = self.head()
        self.approved_plan = copy.deepcopy(f.plan)
        return self.approval

    def write_runtime(self, content="Adapted upstream provider fix preserving downstream behavior\n"):
        f = self.fixture
        f.write("provider.txt", content)
        f.commit("Implement the approved upstream behavior")
        return self.head()

    def cli(self, command, *arguments, success=True):
        f = self.fixture
        result = subprocess.run([sys.executable, str(SCRIPT), command,
                                 "--repo", str(f.repo), "--plan", self.relative_plan,
                                 *map(str, arguments)], env=f.env,
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                check=False)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def finalize(self, success=True):
        result = self.cli("finalize", "--approval-commit", self.approval, success=success)
        if success and self.fixture.git("status", "--porcelain").strip():
            self.fixture.commit("Record manual AI synchronization candidate for CI")
        return result

    def verify(self, success=True, head=None):
        return self.cli("verify", "--base", self.fixture.base,
                        "--head", head or self.head(), success=success)

    def assert_no_release_records(self):
        f = self.fixture
        self.assertFalse(self.history_path.exists())
        self.assertEqual((f.repo / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.2")
        self.assertIn(f"- Commit / 原始提交：`{f.baseline}`", (f.repo / "UPSTREAM.md").read_text())

    def test_planning_only_pr_verifies_without_selecting_items_or_advancing_version(self):
        result = self.verify()
        self.assertIn("planning", result.stdout.lower())
        self.assert_no_release_records()
        self.assertEqual([item["decision"] for item in self.fixture.plan["items"]], ["pending", "pending"])
        self.assertEqual(self.fixture.git("status", "--porcelain").strip(), "")

    def test_same_pr_roundtrip_binds_confirmation_and_actual_code_without_claiming_ci_passed(self):
        f = self.fixture
        self.approve_plan()
        runtime_head = self.write_runtime()
        self.finalize()
        self.verify()

        history = json.loads(self.history_path.read_text())
        self.assertEqual(history["mode"], "manual_ai")
        self.assertEqual(history["state"], "ready_for_ci")
        self.assertEqual(history["approval_commit"], self.approval)
        self.assertEqual(history["plan_sha256"], engine.plan_digest(self.approved_plan))
        self.assertEqual(history["runtime"]["head"], runtime_head)
        self.assertEqual(history["runtime"]["tree"], f.git("rev-parse", f"{runtime_head}^{{tree}}").strip())
        self.assertEqual(history["source"], self.approved_plan["source"])
        self.assertEqual(history["downstream"], self.approved_plan["downstream"])
        self.assertEqual(history["runtime"]["paths"], ["provider.txt"])
        self.assertIn(f.first, json.dumps(history["selected_items"]))
        self.assertNotIn(f.second, json.dumps(history["selected_items"]))
        self.assertNotIn('"status": "passed"', json.dumps(history))
        self.assertEqual(json.loads((f.repo / self.relative_plan).read_text()), self.approved_plan)
        self.assertEqual((f.repo / "VERSION").read_text().strip(), "v1.0.0-rc.26-novamaas.3")
        provenance = (f.repo / "UPSTREAM.md").read_text()
        self.assertIn(f"- Commit / 原始提交：`{f.baseline}`", provenance)
        self.assertIn(f.target, provenance)
        self.assertNotIn("All seven required checks passed", provenance)
        self.assertEqual(f.git("rev-parse", "main").strip(), f.base)
        self.assertEqual(f.git("branch", "--show-current").strip(), "codex/upstream-plan-fixture")
        f.git("merge-base", "--is-ancestor", self.planning_head, self.head())
        finalized_head = self.head()
        self.finalize()
        self.assertEqual(self.head(), finalized_head)

    def test_scoped_manual_migration_does_not_require_an_ai_api_or_a_prebuilt_patch(self):
        self.approve_plan(decision="migrate")
        self.write_runtime()
        self.finalize()
        self.verify()
        self.assertEqual(json.loads((self.fixture.repo / self.relative_plan).read_text()), self.approved_plan)

    def test_append_only_fix_can_refresh_same_pr_records_without_another_version_bump(self):
        f = self.fixture
        self.approve_plan()
        self.write_runtime()
        self.finalize()
        previous_record_head = self.head()
        previous_version = (f.repo / "VERSION").read_text()
        fixed_head = self.write_runtime("Adapted provider fix including the regression correction\n")
        self.verify(success=False)

        self.finalize()
        self.verify()

        self.assertEqual((f.repo / "VERSION").read_text(), previous_version)
        self.assertIn(fixed_head, self.history_path.read_text())
        self.assertEqual(json.loads((f.repo / self.relative_plan).read_text()), self.approved_plan)
        f.git("merge-base", "--is-ancestor", previous_record_head, self.head())
        self.assertEqual(len(list((f.repo / "docs/upstream-sync/history").glob("*.json"))), 1)

    def test_pending_decisions_cannot_authorize_code_or_release_records(self):
        self.approval = self.planning_head
        self.write_runtime()
        self.finalize(success=False)
        self.verify(success=False)
        self.assert_no_release_records()

    def test_confirmation_hash_cannot_be_reused_after_plan_changes(self):
        f = self.fixture
        self.approve_plan()
        f.plan["items"][0]["ai_scope"]["instructions"] = "Different behavior approved in another plan revision."
        self.save_plan()
        f.commit("Change the plan after the cited confirmation")
        self.write_runtime()
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_ai_cannot_select_an_additional_item_after_confirmation(self):
        f = self.fixture
        self.approve_plan()
        f.plan["items"][1].update({
            "decision": "merge",
            "reason": "AI chose another independent improvement without confirmation.",
            "ai_scope": {"paths": ["independent.txt"], "instructions": "Adopt the independent improvement."},
        })
        self.save_plan()
        f.commit("Unexpectedly expand the selected plan")
        self.write_runtime()
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_even_a_new_confirmation_cannot_forge_the_pinned_upstream_target(self):
        f = self.fixture
        self.approve_plan()
        f.plan["source"]["sha"] = f.first
        self.save_plan()
        f.commit("Attempt to approve a different source SHA under the original tag")
        self.approval = self.head()
        self.write_runtime()
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_main_can_be_merged_normally_before_a_fresh_confirmation(self):
        f = self.fixture
        f.git("switch", "main")
        f.write("downstream.txt", "Updated downstream behavior assessed before confirmation\n")
        f.commit("Advance downstream main")
        f.base = self.head()
        f.git("switch", "codex/upstream-plan-fixture")
        f.git("merge", "--no-edit", "main")
        f.plan["downstream"]["base_sha"] = f.base
        self.approve_plan()
        self.write_runtime()
        self.finalize()
        self.verify()
        f.git("merge-base", "--is-ancestor", self.planning_head, self.head())
        f.git("merge-base", "--is-ancestor", f.base, self.head())

    def test_actual_source_changes_cannot_expand_outside_approved_paths(self):
        f = self.fixture
        self.approve_plan()
        self.write_runtime()
        f.write("independent.txt", "Unselected upstream independent improvement\n")
        f.commit("Unexpected unrelated source change")
        self.finalize(success=False)
        self.verify(success=False)
        self.assert_no_release_records()

    def test_planning_commits_cannot_hide_source_changes_before_confirmation(self):
        self.write_runtime("Unapproved source change hidden before plan confirmation\n")
        self.approve_plan()
        self.write_runtime()
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_approved_scope_cannot_authorize_ci_changes(self):
        f = self.fixture
        self.approve_plan(scope=["provider.txt", ".github/workflows/injected.yml"])
        self.write_runtime()
        f.write(".github/workflows/injected.yml", "name: unreviewed source-controlled workflow\n")
        f.commit("Try to modify workflow during migration")
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_approved_runtime_file_cannot_remove_protected_project_identity(self):
        f = self.fixture
        f.git("switch", "main")
        f.write("attribution.txt", "Runtime implementation by QuantumNous\n")
        f.commit("Preserve runtime attribution")
        f.base = self.head()
        f.git("switch", "codex/upstream-plan-fixture")
        f.git("merge", "--no-edit", "main")
        f.plan["downstream"]["base_sha"] = f.base
        self.approve_plan(scope=["provider.txt", "attribution.txt"])
        f.write("attribution.txt", "Runtime implementation downstream\n")
        f.commit("Try to remove project identity from an approved source file")
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_modified_source_version_or_history_cannot_match_deterministic_metadata(self):
        f = self.fixture
        self.approve_plan()
        self.write_runtime()
        self.finalize()
        safe_head = self.head()
        cases = {
            "version": ("VERSION", "v1.0.0-rc.26-novamaas.999\n"),
            "source": ("UPSTREAM.md", (f.repo / "UPSTREAM.md").read_text().replace(f.target, f.first)),
            "history": (str(self.history_path.relative_to(f.repo)), self.history_path.read_text().replace("ready_for_ci", "passed")),
        }
        for label, (path, content) in cases.items():
            with self.subTest(label=label):
                f.git("reset", "--hard", safe_head)
                f.write(path, content)
                f.commit("Forge candidate metadata")
                self.verify(success=False)

    def test_verifier_rejects_an_event_head_different_from_checked_out_pr_head(self):
        self.approve_plan()
        self.write_runtime()
        self.finalize()
        self.verify(success=False, head=self.planning_head)

    def test_protected_metadata_mode_change_cannot_hide_in_source_normalization(self):
        f = self.fixture
        self.approve_plan()
        self.write_runtime()
        (f.repo / "VERSION").chmod(0o755)
        f.commit("Change protected version file mode")
        self.finalize(success=False)
        self.assert_no_release_records()

    def test_generated_metadata_requires_exact_file_mode_when_reused(self):
        f = self.fixture
        self.approve_plan()
        self.write_runtime()
        self.finalize()
        (f.repo / "UPSTREAM.md").chmod(0o755)
        f.commit("Change generated provenance file mode")
        self.verify(success=False)
        self.finalize(success=False)

    def test_committed_conflict_markers_cannot_be_recorded_as_completed_adaptation(self):
        self.approve_plan()
        self.write_runtime("<<<<<<< HEAD\ndownstream behavior\n=======\nupstream behavior\n>>>>>>> source\n")
        self.finalize(success=False)
        self.assert_no_release_records()


if __name__ == "__main__":
    unittest.main()
