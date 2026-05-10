#!/usr/bin/env -S deno run --allow-env --allow-net

import { parseArgs } from "jsr:@std/cli@^1.0/parse-args";

interface Flags {
  server: string;
  inviteCode: string;
}

function parseFlags(): Flags {
  const args = parseArgs(Deno.args, {
    string: ["server", "invite-code"],
    default: {
      server: "http://localhost:54765",
    },
    alias: {
      s: "server",
      i: "invite-code",
    },
  });

  return {
    server: args.server as string,
    inviteCode: (args["invite-code"] as string) ?? "",
  };
}

async function register(
  server: string,
  user: string,
  password: string,
  inviteCode: string,
): Promise<void> {
  const resp = await fetch(`${server}/api/user`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user, password, invite_code: inviteCode }),
  });

  if (resp.status === 201) {
    console.log("Registration successful");
  } else if (resp.status === 409) {
    console.log("User already registered");
  } else {
    const body = await resp.text();
    throw new Error(`Registration failed: ${resp.status} ${body}`);
  }
}

async function login(
  server: string,
  user: string,
  password: string,
): Promise<string> {
  const resp = await fetch(`${server}/api/user/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user, password }),
  });

  if (resp.status !== 200) {
    const body = await resp.text();
    throw new Error(`Login failed: ${resp.status} ${body}`);
  }

  const data = await resp.json();
  return data.token;
}

async function sendMessage(
  server: string,
  token: string,
  content: string,
): Promise<void> {
  const resp = await fetch(`${server}/api/chat/message`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ content }),
  });

  if (resp.status !== 200) {
    const body = await resp.text();
    throw new Error(`Failed to send message: ${resp.status} ${body}`);
  }

  console.log("Message sent successfully.");
}

async function main() {
  const flags = parseFlags();
  const loginEnv = Deno.env.get("GRUNT_LOGIN");

  if (!loginEnv) {
    console.error("Error: GRUNT_LOGIN environment variable not set (expected user:password)");
    Deno.exit(1);
  }

  const [user, password] = loginEnv.split(":");
  if (!user || !password) {
    console.error("Error: GRUNT_LOGIN invalid (expected user:password)");
    Deno.exit(1);
  }

  if (!flags.inviteCode) {
    console.error("Error: --invite-code flag is required");
    Deno.exit(1);
  }

  const message = Deno.args.find((arg) => !arg.startsWith("-"));
  if (!message) {
    console.error("Usage: send.ts <message> [--server URL] [--invite-code CODE]");
    Deno.exit(1);
  }

  try {
    await register(flags.server, user, password, flags.inviteCode);
    const token = await login(flags.server, user, password);
    await sendMessage(flags.server, token, message);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    console.error(`Error: ${message}`);
    Deno.exit(1);
  }
}

await main();
