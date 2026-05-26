import sys
from luma.core.interface.serial import i2c
from luma.oled.device import ssd1306
from PIL import Image, ImageDraw, ImageFont

serial = i2c(port=1, address=0x3C)
device = ssd1306(serial, width=128, height=64)

font_path = "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc"
font = ImageFont.truetype(font_path, 12)

line1 = sys.argv[1] if len(sys.argv) > 1 else ""
line2 = sys.argv[2] if len(sys.argv) > 2 else ""
line3 = sys.argv[3] if len(sys.argv) > 3 else ""
line4 = sys.argv[4] if len(sys.argv) > 4 else ""

image = Image.new("1", (device.width, device.height))
draw = ImageDraw.Draw(image)

draw.text((0, 0), line1, font=font, fill=255)
draw.text((0, 16), line2, font=font, fill=255)
draw.text((0, 32), line3, font=font, fill=255)
draw.text((0, 48), line4, font=font, fill=255)

device.display(image)