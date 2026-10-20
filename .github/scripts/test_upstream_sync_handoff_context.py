"""Verify generated handoff tasks against a real mechanically assessed Git history."""

import copy
import json
from pathlib import Path
import subprocess
import sys
import unittest

import test_upstream_sync as fixtures


SCRIPT = Path(__file__).with_name("upstream_sync_handoff_context.py").resolve()


class UpstreamSyncHandoffContextTests(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.UpstreamSyncIntegrationTests()
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        self.fixture.analyze()
        self.plan_path = self.fixture.plan_path
        self.output = self.plan_path.with_name("AI_HANDOFF.md")

    def cli(self, *arguments, success=True):
        result = subprocess.run([sys.executable, str(SCRIPT), "--plan", str(self.plan_path),
                                 "--output", str(self.output), *arguments],
                                env=self.fixture.env, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, check=False)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def test_handoff_pins_source_and_approval_without_executing_or_selecting_changes(self):
        before = self.fixture.repository_state()
        original_plan = self.plan_path.read_bytes()

        self.cli()

        document = self.output.read_text()
        for value in (self.fixture.first, self.fixture.second, self.fixture.base, self.fixture.baseline,
                      self.fixture.plan["source"]["tag"], self.fixture.plan["downstream"]["proposed_version"]):
            with self.subTest(value=value):
                self.assertIn(value, document)
        self.assertIn("${PR_URL}", document)
        self.assertIn("${APPROVAL_COMMIT}", document)
        self.assertIn("decision 保持 pending", document)
        self.assertIn("manual_ai", document)
        self.assertIn("ready_for_ci", document)
        self.assertIn("CI 不需要 AI API Key", document)
        self.assertEqual(self.plan_path.read_bytes(), original_plan)
        self.assertEqual(self.fixture.repository_state(), before)

    def test_same_pr_handoff_preserves_template_and_requires_green_ci_and_ready_authorization(self):
        url = "https://github.com/example/downstream/pull/75"
        self.cli("--pr-url", url)
        document = self.output.read_text()
        self.assertIn(url, document)
        self.assertIn("gh pr edit --body-file", document)
        self.assertIn("保留仓库模板结构", document)
        self.assertIn("不得勾选需人工确认的复选框", document)
        self.assertIn("仅在用户明确授权", document)
        self.assertIn("最新 HEAD 的所有要求检查通过", document)
        self.assertIn("绝不执行 gh pr merge", document)
        self.assertIn("不 force push", document)
        self.assertIn("不直接推送 main", document)
        self.assertIn("不创建另一个 PR", document)
        self.assertIn("本地不启动 Docker", document)
        self.assertIn("移除 PR 标题中的 WIP 标记", document)
        self.assertIn("ai_scope.paths", document)
        self.assertIn("先读取当前 PR body", document)

    def test_finalize_command_uses_the_plan_batch_and_explicit_approval_commit(self):
        self.cli()
        document = self.output.read_text()
        expected = (
            "python3 .github/scripts/upstream_sync_handoff.py finalize --repo . "
            "--plan docs/upstream-sync/plans/fixture-20261016/plan.json "
            "--approval-commit ${APPROVAL_COMMIT}"
        )
        self.assertIn(expected, document)
        self.assertIn("当前 plan.json 与审批提交的计划内容完全相同", document)
        self.assertIn("每次源码修复后重新运行 finalize", document)
        self.assertIn("不能把已有实现计入新的审批提交", document)
        self.assertIn("正常的 revert 提交", document)
        self.assertIn("也可从最新 main 创建新的评估批次", document)
        self.assertIn("初始 fork 基线", document)
        self.assertIn("不宣称已经完整升级到目标上游版本", document)

    def test_untrusted_ids_urls_and_unpinned_refs_cannot_be_embedded_as_commands(self):
        valid = copy.deepcopy(self.fixture.plan)
        mutations = (
            ("source", "repository", "https://example.invalid/untrusted"),
            ("source", "sha", "main"),
            ("source", "tag", "v1`touch injected`"),
            ("downstream", "base_sha", "HEAD"),
        )
        for section, field, value in mutations:
            with self.subTest(field=field):
                plan = copy.deepcopy(valid)
                plan[section][field] = value
                self.plan_path.write_text(json.dumps(plan), encoding="utf-8")
                self.cli(success=False)
                self.assertFalse(self.output.exists())
        plan = copy.deepcopy(valid)
        plan["items"][0]["id"] = "arbitrary-identity"
        self.plan_path.write_text(json.dumps(plan), encoding="utf-8")
        self.cli(success=False)
        self.assertFalse(self.output.exists())
        self.plan_path.write_text(json.dumps(valid), encoding="utf-8")
        self.cli("--pr-url", "https://github.com/example/downstream/pull/75;echo injected", success=False)
        self.assertFalse(self.output.exists())

    def test_generator_cannot_follow_symlinks_or_write_unrelated_outputs(self):
        self.output = self.fixture.root / "unrelated.md"
        self.cli(success=False)
        self.assertFalse(self.output.exists())
        outside = self.fixture.root / "outside.md"
        outside.write_text("Existing unrelated content\n")
        self.output = self.plan_path.with_name("AI_HANDOFF.md")
        self.output.symlink_to(outside)
        self.cli(success=False)
        self.assertEqual(outside.read_text(), "Existing unrelated content\n")


if __name__ == "__main__":
    unittest.main()
