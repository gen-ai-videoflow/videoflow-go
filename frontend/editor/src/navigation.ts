const router = {
  push: (_path?: string) => {},
  replace: (_path?: string) => {},
  back: () =>
    window.parent.postMessage(
      { type: "videoflow:editor:exit" },
      location.origin,
    ),
};
export const useRouter = () => router;
export const usePathname = () => location.pathname;
export const useParams = () => ({});
