import hashlib
from pathlib import Path
import sys
import tarfile
import zipfile

stage, output, label = Path(sys.argv[1]), Path(sys.argv[2]), sys.argv[3]
output.mkdir(parents=True, exist_ok=True)
archive = output / (label + (".zip" if sys.platform == "win32" else ".tar.gz"))
paths = sorted(stage.rglob("*"))
for path in paths:
    assert not path.is_symlink(), "release staging must contain no symlinks"
    assert path.name != ".env" and ".data" not in path.parts and "node_modules" not in path.parts
if sys.platform == "win32":
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as bundle:
        for path in paths:
            if path.is_file():
                bundle.write(path, Path(label) / path.relative_to(stage))
else:
    with tarfile.open(archive, "w:gz") as bundle:
        bundle.add(stage, arcname=label)
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_name(archive.name + ".sha256").write_text(f"{digest}  {archive.name}\n")
print(archive)
