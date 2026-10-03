"""Build-only: extract Microsoft's signed, hash-pinned redistributable app-locally."""
from pathlib import Path
import hashlib
import struct
import subprocess
import urllib.request
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parent.parent
TOOLS = ROOT / "tools"

def fetch(name, url, expected):
    path = TOOLS / name
    if not path.exists():
        urllib.request.urlretrieve(url, path)
    if hashlib.sha256(path.read_bytes()).hexdigest().lower() != expected.lower():
        raise RuntimeError(f"Checksum mismatch: {name}; refusing changed upstream binary")
    return path

def extract(seven, archive, destination):
    destination.mkdir(parents=True, exist_ok=True)
    subprocess.run([str(seven), "x", str(archive), f"-o{destination}", "-y"], check=True, stdout=subprocess.DEVNULL)

def main():
    TOOLS.mkdir(exist_ok=True)
    bootstrap = fetch("7zr.exe", "https://github.com/ip7z/7zip/releases/download/26.03/7zr.exe", "ad4c82fadcbdf93c03b4fc440f300509c7d60c5c2f4d183e35d9d70d6957037d")
    archive = fetch("7z-extra.7z", "https://github.com/ip7z/7zip/releases/download/26.03/7z2603-extra.7z", "191894e6acb3647ffb69ce630479ff318523b2e2b9890aa7f05c1127c2e59b8f")
    extract(bootstrap, archive, TOOLS / "7zip")
    seven = TOOLS / "7zip" / "x64" / "7za.exe"
    redist = fetch("vc_redist.x64.exe", "https://download.visualstudio.microsoft.com/download/pr/bd1c8d9d-ba95-4eee-bc6e-df1fcc876373/CC0FF0EB1DC3F5188AE6300FAEF32BF5BEEBA4BDD6E8E445A9184072096B713B/VC_redist.x64.exe", "cc0ff0eb1dc3f5188ae6300faef32bf5beeba4bdd6e8e445a9184072096b713b")
    raw = redist.read_bytes()
    cabs = []
    start = 0
    while True:
        offset = raw.find(b"MSCF", start)
        if offset < 0:
            break
        start = offset + 4
        size = struct.unpack_from("<I", raw, offset + 8)[0]
        if 36 <= size <= len(raw) - offset:
            path = TOOLS / f"vc-cab-{offset}.cab"
            path.write_bytes(raw[offset:offset+size])
            dest = TOOLS / "vc-cabs" / path.stem
            extract(seven, path, dest)
            cabs.append(dest)
    manifest = next(path / "0" for path in cabs if (path / "0").exists())
    payloads = [node.attrib for node in ET.fromstring(manifest.read_bytes()).iter() if "FilePath" in node.attrib]
    minimum = next(node for node in payloads if node["FilePath"].lower() == r"packages\vcruntimeminimum_amd64\cab1.cab")
    payload = next(path / minimum["SourcePath"] for path in cabs if (path / minimum["SourcePath"]).exists())
    output = TOOLS / "vc-dlls"
    extract(seven, payload, output)
    target = ROOT / "dist" / "windows" / "postgresql" / "bin"
    for dll in output.glob("*.dll_amd64"):
        (target / dll.name.removesuffix("_amd64")).write_bytes(dll.read_bytes())
    licenses = ROOT / "dist" / "windows" / "licenses"
    licenses.mkdir(parents=True, exist_ok=True)
    license_payload = next(node for node in payloads if node["FilePath"] == "license.rtf")
    license_source = manifest.parent / license_payload["SourcePath"]
    (licenses / "Microsoft-VC-runtime-license.rtf").write_bytes(license_source.read_bytes())
    print("Microsoft VC runtime included app-locally; no system installation required")

if __name__ == "__main__":
    main()
