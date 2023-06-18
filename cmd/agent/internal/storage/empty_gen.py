import argparse
import sys


parser = argparse.ArgumentParser()
parser.add_argument("size", help="empty file size", type=int)
args = parser.parse_args()
start = "%%%#"
end = start[::-1]

print(f"generating empty file of size {args.size} bytes")
print(f"start: {start}, end: {end}")

with open(f"empty.bin", "w") as f:
    f.write(start)
    f.write("\0" * (args.size * 1024))
    f.write(end)
