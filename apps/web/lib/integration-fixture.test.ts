import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

type Fixture = {
  projects: Array<{
    key: string;
    owner: { email: string };
    devices: Array<{
      externalId: string;
      type: string;
      deviceTypeKey: string;
      driverKey: string;
      capabilities: string[];
    }>;
  }>;
};

test("air-ground integration fixture contains two isolated project identities", async () => {
  const fixtureUrl = new URL("../../../test/fixtures/air-ground-projects.json", import.meta.url);
  const fixture = JSON.parse(await readFile(fixtureUrl, "utf8")) as Fixture;
  assert.equal(fixture.projects.length, 2);
  assert.equal(new Set(fixture.projects.map((project) => project.key)).size, 2);
  assert.equal(new Set(fixture.projects.map((project) => project.owner.email)).size, 2);
  assert(fixture.projects.every((project) => project.devices.length > 0));
  assert.equal(
    new Set(fixture.projects.flatMap((project) => project.devices.map((device) => device.externalId))).size,
    2
  );
  assert(fixture.projects.some((project) => project.devices.some((device) => device.type === "drone")));
  assert(fixture.projects.some((project) => project.devices.some((device) => device.type === "ground_robot")));
  assert(fixture.projects.every((project) => project.devices.every((device) => device.deviceTypeKey.length > 0)));
  assert(fixture.projects.every((project) => project.devices.every((device) => device.driverKey.length > 0)));
});
