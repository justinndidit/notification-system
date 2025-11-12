declare interface JwtPayload {
  user_id: string;
  role: string;
}

declare interface JwtRequest extends Request {
  user: JwtPayload;
}

declare interface UserRequest extends Request {
  user?: { userId: string };
  proxy?: (
    targetUrl: string,
    pathPrefix: string,
    addUserHeader?: boolean,
  ) => ReturnType<typeof proxy>;
}
