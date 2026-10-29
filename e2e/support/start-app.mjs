import { spawn, spawnSync } from "node:child_process";
import { mkdirSync } from "node:fs";

const binary = process.platform === "win32" ? ".e2e/avatar.exe" : ".e2e/avatar";

mkdirSync(".e2e", { recursive: true });
const build = spawnSync("go", ["build", "-o", binary, "."], { stdio: "inherit" });
if (build.error) {
  console.error(build.error);
  process.exit(1);
}
if (build.status !== 0) {
  process.exit(build.status ?? 1);
}

const child = spawn(binary, [], { stdio: "inherit", env: process.env });
let stopping = false;

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    if (stopping) {
      return;
    }
    stopping = true;
    child.kill(signal);
  });
}

child.on("exit", (code, signal) => {
  process.exit(signal ? 1 : (code ?? 1));
});
