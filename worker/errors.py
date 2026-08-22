class WorkerError(Exception):
    def __init__(self, message: str, code: str = ""):
        super().__init__(message)
        self.code = code
        self.message = message


class LoggedIn(Exception):
    pass
