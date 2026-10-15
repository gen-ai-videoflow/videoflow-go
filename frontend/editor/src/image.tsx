import React from "react";
export default function Image({
  fill,
  priority,
  unoptimized,
  quality,
  loader,
  ...props
}: any) {
  return <img {...props} />;
}
