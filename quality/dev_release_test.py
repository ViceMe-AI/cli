import importlib.util
from pathlib import Path
import tempfile
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
            self.assertIn("https://s3.viceme.cn/dev/builds/dev-123-1-abcdefabcdef/skills/x",script)
            self.assertIn("https://dev.viceme.cn/api",script)
            self.assertIn("https://s3.viceme.ai/dev/builds/",script)
            self.assertIn("https://s3.viceme.cn/viceme-sdk/1/index.js",script)
    def test_build_identity_rejects_unpinned_names(self):
        with tempfile.TemporaryDirectory() as tmp:
            for build_id,commit in [("dev","a"*40),("dev-1-1-bbbbbbbbbbbb","a"*40),("dev-1-1-aaaaaaaaaaaa","main")]:
                with self.assertRaises(ValueError):
                    mod.build(Path(tmp),Path(tmp)/"out",build_id,commit,[])
