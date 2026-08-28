import { NextResponse } from "next/server";
export async function POST(){
  return NextResponse.json({error:"LEGACY_EVENT_READ_ONLY",issuesHref:"../issues"},{status:410});
}
