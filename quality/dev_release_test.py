import importlib.util
from pathlib import Path
import tempfile
import shutil
import runpy
import unittest

spec=importlib.util.spec_from_file_location("dev_release",Path(__file__).with_name("build-dev-release.py"))
mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod)

class DevArtifactsTest(unittest.TestCase):
    def test_environment_rendering_does_not_modify_production_go_runtime(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            for p in ["skills/use-a-skill/scripts/test.py","release/trial-bootstrap.py.tmpl","internal/update/update.go"]:
                f=root/p;f.parent.mkdir(parents=True,exist_ok=True)
                f.write_text('https://s3.viceme.cn/skills/x https://viceme.cn/api https://s3.viceme.ai/start/x https://s3.viceme.cn/viceme-sdk/1/index.js')
            original=(root/"internal/update/update.go").read_bytes()
            mod.render_dev_assets(root,"dev-123-1-abcdefabcdef")
            self.assertEqual(original,(root/"internal/update/update.go").read_bytes())
            script=(root/"skills/use-a-skill/scripts/test.py").read_text()
            self.assertIn("https://s3.dev.viceme.cn/start/builds/dev-123-1-abcdefabcdef/skills/x",script)
            self.assertIn("https://dev.viceme.cn/api",script)
            self.assertNotIn("https://s3.viceme.ai/dev",script)
            self.assertIn("https://s3.viceme.cn/viceme-sdk/1/index.js",script)
    def test_build_identity_rejects_unpinned_names(self):
        with tempfile.TemporaryDirectory() as tmp:
            for build_id,commit in [("dev","a"*40),("dev-1-1-bbbbbbbbbbbb","a"*40),("dev-1-1-aaaaaaaaaaaa","main")]:
                with self.assertRaises(ValueError):
                    mod.build(Path(tmp),Path(tmp)/"out",build_id,commit,[])

    def test_dev_replica_uses_dev_authority_and_rejects_production(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            target=root/"skills/let-me-make-a-copy/scripts/make_copy.py"
            target.parent.mkdir(parents=True)
            shutil.copyfile(Path(__file__).resolve().parents[1]/target.relative_to(root),target)
            (root/"release").mkdir();(root/"release/trial-bootstrap.py.tmpl").write_text("")
            mod.render_dev_assets(root,"dev-123-1-abcdefabcdef")
            namespace=runpy.run_path(str(target))
            authority=namespace["authority_for_work_url"]("https://dev.viceme.cn/alice/demo.md?product=11111111-1111-4111-8111-111111111111")
            self.assertEqual(authority.api_base_url,"https://dev.viceme.cn/api/v1")
            for host in ("viceme.cn", "viceme.ai", "dev.viceme.ai"):
                with self.assertRaises(namespace["WorkflowError"]):
                    namespace["authority_for_work_url"](f"https://{host}/alice/demo.md")

    def test_dev_trial_bootstrap_has_no_global_choice(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            (root/"release").mkdir()
            source=Path(__file__).resolve().parents[1]/"release/trial-bootstrap.py.tmpl"
            target=root/"release/trial-bootstrap.py.tmpl"
            shutil.copyfile(source,target)
            mod.render_dev_assets(root,"dev-123-1-abcdefabcdef")
            namespace={}
            exec(compile(target.read_text().replace("__RUNTIME_SHA256__", "a"*64),str(target),"exec"),namespace)
            self.assertEqual(list(namespace["SCRIPT_ORIGIN"]),["cn"])
            self.assertEqual(list(namespace["API_ORIGIN"]),["cn"])
