import { consola } from "consola";
import { Permission } from "@prisma/client";

export default defineNitroPlugin(async (nitroApp) => {
  const adminCount = await prisma.user.count({ where: { systemAdmin: true } });
  if (adminCount === 0) {
    prisma.user
      .create({
        data: {
          username: process.env.ADMIN_USERNAME || "admin",
          password: await hashPassword(process.env.ADMIN_PASSWORD || "admin"),
          email: process.env.ADMIN_EMAIL,
          systemAdmin: true,
        },
      })
      .then(() => {
        consola.success("Default admin user created successfully");
      });
  }

  // Check and create default Owner role if no roles exist
  const roleCount = await prisma.role.count();
  if (roleCount === 0) {
    consola.info("No roles found in the database. Creating default Owner role...");
    try {
      await prisma.role.create({
        data: {
          name: "Owner",
          description: "Default owner role with all permissions",
          permissions: [Permission.READ, Permission.WRITE, Permission.DELETE],
          // teamId will be null by default, making it a global role
        },
      });
      consola.success("Default Owner role created successfully");
    } catch (error) {
      consola.error("Failed to create default Owner role:", error);
    }
  }
});
