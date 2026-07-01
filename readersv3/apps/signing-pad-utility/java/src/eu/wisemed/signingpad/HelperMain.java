package eu.wisemed.signingpad;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

public final class HelperMain {
    public static void main(String[] args) throws Exception {
        String input = readAll();
        Map<String, String> request = SimpleJson.parseObject(input);
        String action = trim(request.get("action"));
        if (action.isEmpty()) {
            write(SimpleJson.renderObject(error("missing action")));
            return;
        }
        switch (action) {
            case "health":
                write(SimpleJson.renderObject(health()));
                return;
            case "capture":
                write(SimpleJson.renderObject(captureStub(request)));
                return;
            default:
                write(SimpleJson.renderObject(error("unsupported action: " + action)));
        }
    }

    private static Map<String, String> health() {
        Map<String, String> out = new LinkedHashMap<>();
        out.put("ok", "true");
        out.put("status", "ready");
        out.put("message", "java helper available");
        out.put("javaVersion", System.getProperty("java.version", ""));
        out.put("timestamp", Instant.now().toString());
        return out;
    }

    private static Map<String, String> captureStub(Map<String, String> request) {
        Map<String, String> out = new LinkedHashMap<>();
        out.put("ok", "false");
        out.put("status", "not_implemented");
        out.put("deviceType", trim(request.get("deviceType")));
        out.put("sdkMode", trim(request.get("sdkMode")));
        out.put("message", "SDK bridge not implemented yet. Add vendor JARs and adapter code for SigPadPureFacade/SigPadFacade.");
        out.put("timestamp", Instant.now().toString());
        return out;
    }

    private static Map<String, String> error(String message) {
        Map<String, String> out = new LinkedHashMap<>();
        out.put("ok", "false");
        out.put("status", "error");
        out.put("message", message);
        out.put("timestamp", Instant.now().toString());
        return out;
    }

    private static void write(String value) {
        System.out.print(value);
        System.out.flush();
    }

    private static String readAll() throws IOException {
        StringBuilder sb = new StringBuilder();
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8))) {
            String line;
            while ((line = reader.readLine()) != null) {
                sb.append(line);
            }
        }
        return sb.toString();
    }

    private static String trim(String value) {
        return value == null ? "" : value.trim();
    }
}
