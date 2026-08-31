export class WeKnoraError extends Error {
  statusCode: number;
  code: string;
  constructor(message: string, statusCode: number, code: string) {
    super(message);
    this.name = 'WeKnoraError';
    this.statusCode = statusCode;
    this.code = code;
  }
}

export class UnauthorizedError extends WeKnoraError {
  constructor(message = 'Unauthorized') {
    super(message, 401, 'unauthorized');
  }
}

export class ForbiddenError extends WeKnoraError {
  constructor(message = 'Forbidden') {
    super(message, 403, 'forbidden');
  }
}

export class NotFoundError extends WeKnoraError {
  constructor(message = 'Not Found') {
    super(message, 404, 'not_found');
  }
}

export class RateLimitError extends WeKnoraError {
  constructor(message = 'Rate Limited') {
    super(message, 429, 'rate_limited');
  }
}
