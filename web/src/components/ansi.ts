// Bộ giải mã mã màu ANSI sang mảng segments hiển thị UI (docs/03 SCR-06).
// Hỗ trợ SGR 0 (reset), 1 (bold), 30-37 / 90-97 (màu cơ bản và sáng), 39 (bỏ màu).
// Loại bỏ mọi chuỗi ESC khác (CSI, OSC...), không để lọt ký tự \x1b.

export interface AnsiSegment {
  text: string;
  color?: string;
  bold?: boolean;
}

const COLOR_MAP: Record<number, string> = {
  30: 'black',
  31: 'red',
  32: 'green',
  33: 'yellow',
  34: 'blue',
  35: 'magenta',
  36: 'cyan',
  37: 'white',
  90: 'black',
  91: 'red',
  92: 'green',
  93: 'yellow',
  94: 'blue',
  95: 'magenta',
  96: 'cyan',
  97: 'white',
};

// Khớp CSI: \x1b\[([0-9:;<=>?]*[ -/]*)([@-~])
// Khớp OSC: \x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?
// Khớp các chuỗi ESC 2 ký tự: \x1b[()#%][A-Za-z0-9] | \x1b[@-Z\\-_]
// Khớp ký tự \x1b lẻ loi
const ANSI_REGEX = /\x1b\[([0-9:;<=>?]*[ -/]*)([@-~])|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?|\x1b[()#%][A-Za-z0-9]|\x1b[@-Z\\-_]|\x1b/g;

export function ansiToSegments(text: string): AnsiSegment[] {
  if (!text) return [];

  const segments: AnsiSegment[] = [];
  let currentColor: string | undefined = undefined;
  let isBold = false;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  function pushText(rawText: string) {
    const clean = rawText.replace(/\x1b/g, '');
    if (!clean) return;

    const last = segments[segments.length - 1];
    if (last && last.color === currentColor && Boolean(last.bold) === isBold) {
      last.text += clean;
    } else {
      const seg: AnsiSegment = { text: clean };
      if (currentColor) seg.color = currentColor;
      if (isBold) seg.bold = true;
      segments.push(seg);
    }
  }

  ANSI_REGEX.lastIndex = 0;
  while ((match = ANSI_REGEX.exec(text)) !== null) {
    if (match.index > lastIndex) {
      pushText(text.slice(lastIndex, match.index));
    }
    lastIndex = match.index + match[0].length;

    // Kiểm tra xem có phải CSI kết thúc bằng 'm' (SGR) không
    if (match[2] === 'm') {
      const rawParams = match[1] ?? '';
      const codes = rawParams.length === 0
        ? [0]
        : rawParams.split(';').map((p) => (p === '' ? 0 : Number.parseInt(p, 10)));

      for (const code of codes) {
        if (Number.isNaN(code)) continue;

        if (code === 0) {
          currentColor = undefined;
          isBold = false;
        } else if (code === 1) {
          isBold = true;
        } else if (code === 22) {
          isBold = false;
        } else if (code === 39) {
          currentColor = undefined;
        } else if (COLOR_MAP[code]) {
          currentColor = COLOR_MAP[code];
        }
      }
    }
  }

  if (lastIndex < text.length) {
    pushText(text.slice(lastIndex));
  }

  return segments;
}
