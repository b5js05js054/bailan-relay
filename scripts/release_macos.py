#!/usr/bin/env python3
"""一键发布 macOS 包：提交 -> 推送 -> 等 GitHub Actions 出包 -> 下载到本机。

用法（在仓库任意目录执行）：
    python scripts/release_macos.py                      # 提交改动并推送，沿用源码版本号
    python scripts/release_macos.py --version 1.2.9      # 同时打 tag v1.2.4，出包版本号为 1.2.4
    python scripts/release_macos.py -m "修复 xxx"         # 自定义提交说明
    python scripts/release_macos.py --no-wait            # 只推送，不等待/不下载

GitHub token（等待与下载需要；仓库为私有）：
    环境变量 BAILAN_GH_TOKEN，或文件 .bailan/github_token（已被 .gitignore 忽略，不会入库）。
    推荐 fine-grained token：仅授权本仓库，权限 Actions: Read-only。

产出：artifacts/macos/<版本或提交>/ 下的两个 zip（标准版 / 客户版），并打印 SHA256 供后台填写。
"""
from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
import zipfile
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW_FILE = "build-macos.yml"
BRANCH = "main"
API = "https://api.github.com"
VERSION_RE = re.compile(r"^\d+(?:\.\d+){0,3}$")


def git(*args: str, check: bool = True) -> str:
    """在仓库根目录执行 git。输入：git 参数；输出：去掉首尾空白的 stdout。失败时退出脚本。"""
    proc = subprocess.run(["git", *args], cwd=ROOT, capture_output=True, text=True, encoding="utf-8", errors="replace")
    if check and proc.returncode != 0:
        sys.exit(f"git {' '.join(args)} 失败：\n{proc.stderr.strip() or proc.stdout.strip()}")
    return proc.stdout.strip()


def git_push(*refs: str) -> None:
    """推送指定 ref 到 origin。GitHub SSH 偶发拒绝，最多重试 3 次。输入：ref 列表。"""
    for attempt in range(1, 4):
        proc = subprocess.run(["git", "push", "origin", *refs], cwd=ROOT, capture_output=True, text=True, encoding="utf-8", errors="replace")
        if proc.returncode == 0:
            print(f"已推送：{' '.join(refs)}")
            return
        print(f"推送失败（第 {attempt} 次），稍后重试：{proc.stderr.strip().splitlines()[-1:]}")
        time.sleep(3)
    sys.exit("推送连续失败，请检查网络或 SSH 钥匙。")


def repo_slug() -> str:
    """从 origin 地址解析 owner/repo。输出：如 b5js05js054/bailan-relay。"""
    url = git("remote", "get-url", "origin")
    m = re.search(r"github\.com[:/](.+?)(?:\.git)?$", url)
    if not m:
        sys.exit(f"origin 不是 GitHub 地址：{url}")
    return m.group(1)


def load_token() -> str:
    """读取 GitHub token。输出：token；未配置时返回空串。"""
    token = os.environ.get("BAILAN_GH_TOKEN", "").strip()
    path = ROOT / ".bailan" / "github_token"
    if not token and path.is_file():
        token = path.read_text(encoding="utf-8").strip()
    return token


def api(token: str, path: str, raw: bool = False):
    """调用 GitHub REST API。输入：token、以 / 开头的路径；raw=True 时返回字节（会跟随重定向）。
    Authorization 用 unredirected header，避免被转发到产物下载的预签名地址。"""
    req = urllib.request.Request(API + path)
    req.add_unredirected_header("Authorization", f"Bearer {token}")
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("X-GitHub-Api-Version", "2022-11-28")
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            body = resp.read()
    except urllib.error.HTTPError as exc:
        if exc.code in (401, 403, 404):
            sys.exit(f"GitHub API {exc.code}：token 无效或缺少本仓库 Actions 读权限（{path}）")
        raise
    return body if raw else json.loads(body)


def commit_changes(message: str) -> None:
    """有改动时全部提交。输入：提交说明。"""
    if not git("status", "--porcelain"):
        print("工作区无改动，直接推送现有提交。")
        return
    git("add", "-A")
    print(git("status", "--short"))
    git("commit", "-q", "-m", message)
    print(f"已提交：{git('log', '--oneline', '-1')}")


def ensure_tag_free(tag: str) -> None:
    """确认本地与远端都没有该 tag，避免覆盖已发布版本。输入：tag 名。"""
    if git("rev-parse", "-q", "--verify", f"refs/tags/{tag}", check=False):
        sys.exit(f"本地已存在 tag {tag}，请换一个版本号。")
    if git("ls-remote", "--tags", "origin", tag, check=False):
        sys.exit(f"远端已存在 tag {tag}，请换一个版本号。")


def find_run(token: str, slug: str, sha: str, ref_name: str) -> dict:
    """等待并返回对应提交与分支/tag 的 workflow run。输入：sha、分支或 tag 名；输出：run 对象。"""
    path = f"/repos/{slug}/actions/workflows/{WORKFLOW_FILE}/runs?head_sha={sha}&per_page=20"
    deadline = time.time() + 180
    while time.time() < deadline:
        runs = [r for r in api(token, path).get("workflow_runs", []) if r.get("head_branch") == ref_name]
        if runs:
            return max(runs, key=lambda r: r["id"])
        time.sleep(5)
    sys.exit("3 分钟内没等到 CI 运行，请到 GitHub Actions 页面确认 workflow 是否被触发。")


def wait_run(token: str, slug: str, run: dict) -> dict:
    """轮询 run 直到结束（最长 40 分钟）。输入：run 对象；输出：结束后的 run 对象。"""
    print(f"CI 运行中：{run['html_url']}")
    start, last = time.time(), ""
    while time.time() - start < 40 * 60:
        run = api(token, f"/repos/{slug}/actions/runs/{run['id']}")
        status = f"{run['status']}/{run.get('conclusion') or '-'}"
        if status != last:
            print(f"  [{int(time.time() - start):>4}s] {status}")
            last = status
        if run["status"] == "completed":
            return run
        time.sleep(15)
    sys.exit(f"等待超时，请手动查看：{run['html_url']}")


def print_failure(token: str, slug: str, run: dict) -> None:
    """打印失败步骤与日志末尾，方便直接贴给维护者。输入：已结束的 run。"""
    jobs = api(token, f"/repos/{slug}/actions/runs/{run['id']}/jobs").get("jobs", [])
    for job in jobs:
        if job.get("conclusion") == "success":
            continue
        failed = [s["name"] for s in job.get("steps", []) if s.get("conclusion") == "failure"]
        print(f"\n失败任务：{job['name']}，失败步骤：{', '.join(failed) or '未知'}")
        try:
            log = api(token, f"/repos/{slug}/actions/jobs/{job['id']}/logs", raw=True).decode("utf-8", "replace")
            print("---- 日志末尾 60 行 ----")
            print("\n".join(log.splitlines()[-60:]))
        except Exception as exc:  # 日志拉取失败不影响结论
            print(f"(日志拉取失败：{exc})")
    sys.exit(f"\nCI 失败：{run['html_url']}")


def download_artifacts(token: str, slug: str, run: dict, out_dir: Path) -> None:
    """下载 run 的全部产物并解出内层 zip。输入：run、输出目录。打印每个包的 SHA256。"""
    artifacts = api(token, f"/repos/{slug}/actions/runs/{run['id']}/artifacts").get("artifacts", [])
    if not artifacts:
        sys.exit("CI 成功但没有产物，请检查 workflow 的 upload-artifact 步骤。")
    out_dir.mkdir(parents=True, exist_ok=True)
    for art in artifacts:
        data = api(token, f"/repos/{slug}/actions/artifacts/{art['id']}/zip", raw=True)
        with zipfile.ZipFile(io.BytesIO(data)) as outer:
            for name in outer.namelist():
                target = out_dir / Path(name).name
                target.write_bytes(outer.read(name))
                digest = hashlib.sha256(target.read_bytes()).hexdigest()
                print(f"  {target}\n    SHA256 {digest}")


def main() -> None:
    parser = argparse.ArgumentParser(description="提交并推送，等 GitHub Actions 打包 macOS 版后下载到本机。")
    parser.add_argument("-m", "--message", help="提交说明（默认自动生成）")
    parser.add_argument("--version", help="发布版本号，如 1.2.4；会打 tag v1.2.4 并注入程序")
    parser.add_argument("--no-wait", action="store_true", help="只推送，不等待 CI、不下载")
    parser.add_argument("--out", default="artifacts/macos", help="下载目录（相对仓库根，默认 artifacts/macos）")
    args = parser.parse_args()

    if args.version and not VERSION_RE.match(args.version.lstrip("v")):
        sys.exit("版本号格式应为 1.2.4")
    version = args.version.lstrip("v") if args.version else ""
    branch = git("rev-parse", "--abbrev-ref", "HEAD")
    if branch != BRANCH:
        sys.exit(f"当前在分支 {branch}，请切到 {BRANCH} 后再发布。")

    token = "" if args.no_wait else load_token()
    if not args.no_wait and not token:
        sys.exit("未找到 GitHub token：请设置环境变量 BAILAN_GH_TOKEN 或写入 .bailan/github_token（或加 --no-wait 只推送）。")

    tag = f"v{version}" if version else ""
    if tag:
        ensure_tag_free(tag)
    default_msg = f"发布 macOS {tag}" if tag else f"更新 macOS 构建 {datetime.now():%Y-%m-%d %H:%M}"
    commit_changes(args.message or default_msg)
    sha = git("rev-parse", "HEAD")
    if tag:
        git("tag", "-a", tag, "-m", f"release {tag}")
        git_push(BRANCH, tag)
    else:
        git_push(BRANCH)

    if args.no_wait:
        print(f"已推送 {sha[:7]}，CI 会自动打包：https://github.com/{repo_slug()}/actions")
        return

    slug = repo_slug()
    run = wait_run(token, slug, find_run(token, slug, sha, tag or BRANCH))
    if run.get("conclusion") != "success":
        print_failure(token, slug, run)
    out_dir = ROOT / args.out / (tag or sha[:7])
    print(f"CI 成功，下载产物到 {out_dir}")
    download_artifacts(token, slug, run, out_dir)
    print("\n完成。上传 zip 到直链后，在后台「发布配置」填写 macOS 地址与上面的 SHA256。")


if __name__ == "__main__":
    main()
