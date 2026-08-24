import { NextRequest, NextResponse } from "next/server";

import { createDeviceAdapter, listDeviceAdapters } from "@/lib/device-adapters";

export async function GET(_request: NextRequest, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    return NextResponse.json(await listDeviceAdapters(Number(id)));
  } catch {
    return NextResponse.json({ error: "Unable to access device adapters" }, { status: 403 });
  }
}

export async function POST(request: NextRequest, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    const adapter = await createDeviceAdapter(
      Number(id),
      await request.json(),
      request.headers.get("x-request-id")
    );
    return NextResponse.json(adapter, { status: 201 });
  } catch {
    return NextResponse.json({ error: "Invalid or unauthorized device adapter request" }, { status: 400 });
  }
}
