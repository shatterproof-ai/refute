package ai.shatterproof.refute;

import java.io.IOException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

public final class RefuteTool {
  private RefuteTool() {}

  public static void main(String[] args) throws IOException, InterruptedException {
    List<String> command = new ArrayList<>();
    if (args.length > 0 && args[0].equals("sync")) {
      command.add("refute-tool");
      command.add("sync");
    } else {
      command.add(".refute/bin/refute");
      if (args.length > 0 && args[0].equals("--")) {
        command.addAll(Arrays.asList(args).subList(1, args.length));
      } else {
        command.addAll(Arrays.asList(args));
      }
    }
    Process process = new ProcessBuilder(command).inheritIO().start();
    System.exit(process.waitFor());
  }
}
