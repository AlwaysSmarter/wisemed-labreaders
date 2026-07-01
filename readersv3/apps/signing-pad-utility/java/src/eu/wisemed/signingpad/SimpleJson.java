package eu.wisemed.signingpad;

import java.util.LinkedHashMap;
import java.util.Map;

final class SimpleJson {
    private SimpleJson() {}

    static Map<String, String> parseObject(String raw) {
        Map<String, String> out = new LinkedHashMap<>();
        if (raw == null) {
            return out;
        }
        String s = raw.trim();
        if (s.isEmpty() || s.equals("{}")) {
            return out;
        }
        if (s.startsWith("{")) {
            s = s.substring(1);
        }
        if (s.endsWith("}")) {
            s = s.substring(0, s.length() - 1);
        }
        int index = 0;
        while (index < s.length()) {
            int keyStart = findNextQuote(s, index);
            if (keyStart < 0) {
                break;
            }
            int keyEnd = findClosingQuote(s, keyStart + 1);
            if (keyEnd < 0) {
                break;
            }
            String key = unescape(s.substring(keyStart + 1, keyEnd));
            int colon = s.indexOf(':', keyEnd + 1);
            if (colon < 0) {
                break;
            }
            int valueStart = skipWhitespace(s, colon + 1);
            String value;
            int nextIndex;
            if (valueStart < s.length() && s.charAt(valueStart) == '"') {
                int valueEnd = findClosingQuote(s, valueStart + 1);
                if (valueEnd < 0) {
                    break;
                }
                value = unescape(s.substring(valueStart + 1, valueEnd));
                nextIndex = valueEnd + 1;
            } else {
                int comma = findNextComma(s, valueStart);
                int end = comma >= 0 ? comma : s.length();
                value = s.substring(valueStart, end).trim();
                nextIndex = end;
            }
            out.put(key, value);
            int comma = s.indexOf(',', nextIndex);
            if (comma < 0) {
                break;
            }
            index = comma + 1;
        }
        return out;
    }

    static String renderObject(Map<String, String> values) {
        StringBuilder sb = new StringBuilder();
        sb.append('{');
        boolean first = true;
        for (Map.Entry<String, String> entry : values.entrySet()) {
            if (!first) {
                sb.append(',');
            }
            first = false;
            sb.append('"').append(escape(entry.getKey())).append('"').append(':');
            String value = entry.getValue();
            if ("true".equals(value) || "false".equals(value) || "null".equals(value)) {
                sb.append(value);
            } else {
                sb.append('"').append(escape(value == null ? "" : value)).append('"');
            }
        }
        sb.append('}');
        return sb.toString();
    }

    private static int skipWhitespace(String s, int index) {
        while (index < s.length() && Character.isWhitespace(s.charAt(index))) {
            index++;
        }
        return index;
    }

    private static int findNextQuote(String s, int start) {
        for (int i = start; i < s.length(); i++) {
            if (s.charAt(i) == '"') {
                return i;
            }
        }
        return -1;
    }

    private static int findClosingQuote(String s, int start) {
        boolean escaped = false;
        for (int i = start; i < s.length(); i++) {
            char ch = s.charAt(i);
            if (escaped) {
                escaped = false;
                continue;
            }
            if (ch == '\\') {
                escaped = true;
                continue;
            }
            if (ch == '"') {
                return i;
            }
        }
        return -1;
    }

    private static int findNextComma(String s, int start) {
        boolean inString = false;
        boolean escaped = false;
        for (int i = start; i < s.length(); i++) {
            char ch = s.charAt(i);
            if (escaped) {
                escaped = false;
                continue;
            }
            if (ch == '\\') {
                escaped = true;
                continue;
            }
            if (ch == '"') {
                inString = !inString;
                continue;
            }
            if (!inString && ch == ',') {
                return i;
            }
        }
        return -1;
    }

    private static String escape(String value) {
        return value.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r");
    }

    private static String unescape(String value) {
        return value.replace("\\n", "\n").replace("\\r", "\r").replace("\\\"", "\"").replace("\\\\", "\\");
    }
}
