"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useAccount = void 0;
const react_1 = require("react");
const account_1 = require("../contexts/account");
const useAccount = () => {
    const account = (0, react_1.useContext)(account_1.AccountContext);
    return account;
};
exports.useAccount = useAccount;
exports.default = exports.useAccount;
//# sourceMappingURL=useAccount.js.map