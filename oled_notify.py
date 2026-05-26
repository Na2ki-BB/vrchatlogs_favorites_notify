import sys
from luma.core.interface.serial import i2c
from luma.core.render import canvas
from luma.oled.device import ssd1306

serial = i2c(port=1, address=0x3C)
device = ssd1306(serial, width=128, height=64)

# 引数を受け取る
# 例: python3 oled_notify.py ONLINE FriendName 15:04
line1 = sys.argv[1] if len(sys.argv) > 1 else ""
line2 = sys.argv[2] if len(sys.argv) > 2 else ""
line3 = sys.argv[3] if len(sys.argv) > 3 else ""
line4 = sys.argv[4] if len(sys.argv) > 4 else ""

with canvas(device) as draw:
    draw.text((0, 0), line1, fill="white")
    draw.text((0, 16), line2, fill="white")
    draw.text((0, 32), line3, fill="white")
    draw.text((0, 48), line4, fill="white")