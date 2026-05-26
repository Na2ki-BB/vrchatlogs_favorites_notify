import os
import time
from luma.core.interface.serial import i2c
from luma.core.render import canvas
from luma.oled.device import ssd1306
from PIL import ImageFont

DISPLAY_FILE = "/tmp/vrchat_oled.txt"

serial = i2c(port=1, address=0x3C)
device = ssd1306(serial, width=128, height=64)

font_path = "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc"
font = ImageFont.truetype(font_path, 12)

last_content = None

def read_lines():
    if not os.path.exists(DISPLAY_FILE):
        return ["VRChat Notify", "Waiting...", "", ""]

    with open(DISPLAY_FILE, "r", encoding="utf-8") as f:
        lines = f.read().splitlines()

    while len(lines) < 4:
        lines.append("")

    return lines[:4]

def show(lines):
    with canvas(device) as draw:
        draw.text((0, 0), lines[0], font=font, fill="white")
        draw.text((0, 16), lines[1], font=font, fill="white")
        draw.text((0, 32), lines[2], font=font, fill="white")
        draw.text((0, 48), lines[3], font=font, fill="white")

print("OLED daemon started.")

show(["VRChat Notify", "Waiting...", "", ""])

while True:
    lines = read_lines()
    content = "\n".join(lines)

    if content != last_content:
        show(lines)
        last_content = content

    time.sleep(0.2)