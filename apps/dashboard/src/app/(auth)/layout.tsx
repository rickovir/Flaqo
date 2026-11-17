export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  // The sign in page has no navigation, so it renders without the
  // application shell.
  return <>{children}</>;
}
