-- CreateTable
CREATE TABLE "POI" (
    "id" SERIAL NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "latitude" DOUBLE PRECISION NOT NULL,
    "longitude" DOUBLE PRECISION NOT NULL,
    "projectId" INTEGER NOT NULL,

    CONSTRAINT "POI_pkey" PRIMARY KEY ("id")
);

-- AddForeignKey
ALTER TABLE "POI" ADD CONSTRAINT "POI_projectId_fkey" FOREIGN KEY ("projectId") REFERENCES "Project"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
