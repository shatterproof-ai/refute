package ai.shatterproof.refute;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

public final class RefuteTool {
  private static final String LOCKFILE = "refute.lock.json";

  private RefuteTool() {}

  public static void main(String[] args) throws IOException, InterruptedException {
    List<String> command = new ArrayList<>();
    boolean delegatingSync = args.length > 0 && args[0].equals("sync");
    if (delegatingSync) {
      // Delegate sync to the canonical refute-tool, which performs the walk-up
      // and the actual sync.
      command.add("refute-tool");
      command.add("sync");
    } else {
      // Resolve .refute/bin/refute from the lockfile directory (walk up) so the
      // shim works from any subdirectory, not just the project root.
      command.add(projectRoot().resolve(Paths.get(".refute", "bin", "refute")).toString());
      if (args.length > 0 && args[0].equals("--")) {
        command.addAll(Arrays.asList(args).subList(1, args.length));
      } else {
        command.addAll(Arrays.asList(args));
      }
    }
    try {
      Process process = new ProcessBuilder(command).inheritIO().start();
      // waitFor() returns 128+signal on Unix for signal deaths, so exit codes
      // (including signal deaths) propagate without collapsing to success.
      System.exit(process.waitFor());
    } catch (IOException err) {
      if (delegatingSync) {
        System.err.println(
            "refute-tool not found on PATH; the jvm shim delegates `sync` to refute-tool.\n"
                + "Install a refute package-manager shim that ships refute-tool (npm:"
                + " @shatterproof-ai/refute-tool, pip: refute-tool), or run `refute-tool sync` from a"
                + " refute checkout. See INSTALL.md.");
      } else {
        System.err.println(
            ".refute/bin/refute is missing; run `refute-tool sync` first (" + err.getMessage() + ").");
      }
      System.exit(127);
    }
  }

  // projectRoot walks up from the working directory to the directory containing
  // the lockfile. Falls back to the working directory when no lockfile is found.
  private static Path projectRoot() {
    Path cwd = Paths.get("").toAbsolutePath();
    for (Path dir = cwd; dir != null; dir = dir.getParent()) {
      if (Files.isRegularFile(dir.resolve(LOCKFILE))) {
        return dir;
      }
    }
    return cwd;
  }
}
