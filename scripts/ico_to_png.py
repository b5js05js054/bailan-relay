# CI/本地（Mac）通用：从 icon.ico 中提取内嵌的 256x256 PNG 帧用于生成 icns。
# 输入：icon.ico 路径、输出 PNG 路径。
import struct
import sys


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: ico_to_png.py <input.ico> <output.png>")
    with open(sys.argv[1], "rb") as f:
        data = f.read()
    reserved, icon_type, count = struct.unpack("<HHH", data[0:6])
    if icon_type != 1:
        sys.exit("not an ICO file")
    for i in range(count):
        entry = data[6 + i * 16 : 6 + (i + 1) * 16]
        _, _, _, _, _, _, size, offset = struct.unpack("<BBBBHHII", entry)
        blob = data[offset : offset + size]
        if blob[:8] == b"\x89PNG\r\n\x1a\n":
            with open(sys.argv[2], "wb") as out:
                out.write(blob)
            print(f"extracted {size} byte PNG frame #{i}")
            return
    sys.exit("no PNG frame found in ICO")


if __name__ == "__main__":
    main()
