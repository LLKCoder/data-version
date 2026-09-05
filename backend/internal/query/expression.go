package query

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func evalExpression(source string, row map[string]any) (any, error) {
	expression, err := parser.ParseExpr(source)
	if err != nil {
		return nil, fmt.Errorf("表达式语法无效: %w", err)
	}
	return evalAST(expression, row)
}

func evalAST(node ast.Expr, row map[string]any) (any, error) {
	switch expression := node.(type) {
	case *ast.ParenExpr:
		return evalAST(expression.X, row)
	case *ast.Ident:
		switch expression.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		}
		value, ok := row[expression.Name]
		if !ok {
			return nil, fmt.Errorf("字段不存在: %s", expression.Name)
		}
		return value, nil
	case *ast.BasicLit:
		switch expression.Kind {
		case token.STRING:
			return strconv.Unquote(expression.Value)
		case token.INT:
			return strconv.ParseInt(expression.Value, 0, 64)
		case token.FLOAT:
			return strconv.ParseFloat(expression.Value, 64)
		}
	case *ast.UnaryExpr:
		value, err := evalAST(expression.X, row)
		if err != nil {
			return nil, err
		}
		switch expression.Op.String() {
		case "!":
			return !truthy(value), nil
		case "+":
			if number, ok := toFloat(value); ok {
				return number, nil
			}
		case "-":
			if number, ok := toFloat(value); ok {
				return -number, nil
			}
		}
		return nil, fmt.Errorf("不支持的一元运算: %s", expression.Op)
	case *ast.BinaryExpr:
		return evalBinary(expression, row)
	case *ast.CallExpr:
		return evalCall(expression, row)
	default:
		return nil, fmt.Errorf("不支持的表达式节点: %T", node)
	}
	return nil, fmt.Errorf("不支持的表达式: %s", sourceForNode(node))
}

func evalBinary(expression *ast.BinaryExpr, row map[string]any) (any, error) {
	left, err := evalAST(expression.X, row)
	if err != nil {
		return nil, err
	}
	if expression.Op.String() == "&&" && !truthy(left) {
		return false, nil
	}
	if expression.Op.String() == "||" && truthy(left) {
		return true, nil
	}
	right, err := evalAST(expression.Y, row)
	if err != nil {
		return nil, err
	}
	switch expression.Op.String() {
	case "&&":
		return truthy(left) && truthy(right), nil
	case "||":
		return truthy(left) || truthy(right), nil
	case "==":
		return reflect.DeepEqual(left, right) || numericEqual(left, right), nil
	case "!=":
		return !(reflect.DeepEqual(left, right) || numericEqual(left, right)), nil
	case ">", ">=", "<", "<=":
		leftNumber, leftOK := toFloat(left)
		rightNumber, rightOK := toFloat(right)
		if leftOK && rightOK {
			switch expression.Op.String() {
			case ">":
				return leftNumber > rightNumber, nil
			case ">=":
				return leftNumber >= rightNumber, nil
			case "<":
				return leftNumber < rightNumber, nil
			case "<=":
				return leftNumber <= rightNumber, nil
			}
		}
		leftString, leftOK := left.(string)
		rightString, rightOK := right.(string)
		if leftOK && rightOK {
			switch expression.Op.String() {
			case ">":
				return leftString > rightString, nil
			case ">=":
				return leftString >= rightString, nil
			case "<":
				return leftString < rightString, nil
			case "<=":
				return leftString <= rightString, nil
			}
		}
		return nil, errorsForOperator(expression.Op.String())
	case "+":
		if leftString, ok := left.(string); ok {
			if rightString, ok := right.(string); ok {
				return leftString + rightString, nil
			}
		}
		leftNumber, leftOK := toFloat(left)
		rightNumber, rightOK := toFloat(right)
		if leftOK && rightOK {
			return leftNumber + rightNumber, nil
		}
	case "-", "*", "/", "%":
		leftNumber, leftOK := toFloat(left)
		rightNumber, rightOK := toFloat(right)
		if leftOK && rightOK {
			switch expression.Op.String() {
			case "-":
				return leftNumber - rightNumber, nil
			case "*":
				return leftNumber * rightNumber, nil
			case "/":
				if rightNumber == 0 {
					return nil, fmt.Errorf("除数不能为零")
				}
				return leftNumber / rightNumber, nil
			case "%":
				if rightNumber == 0 {
					return nil, fmt.Errorf("除数不能为零")
				}
				return math.Mod(leftNumber, rightNumber), nil
			}
		}
	}
	return nil, errorsForOperator(expression.Op.String())
}

func evalCall(expression *ast.CallExpr, row map[string]any) (any, error) {
	identifier, ok := expression.Fun.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("只允许调用内置计算函数")
	}
	values := make([]any, len(expression.Args))
	for index, argument := range expression.Args {
		value, err := evalAST(argument, row)
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	switch strings.ToLower(identifier.Name) {
	case "coalesce", "ifnull":
		for _, value := range values {
			if value != nil && value != "" {
				return value, nil
			}
		}
		return nil, nil
	case "abs":
		if len(values) != 1 {
			return nil, errorsForArgumentCount(identifier.Name)
		}
		number, ok := toFloat(values[0])
		if !ok {
			return nil, errorsForType(identifier.Name)
		}
		return math.Abs(number), nil
	case "round":
		if len(values) < 1 || len(values) > 2 {
			return nil, errorsForArgumentCount(identifier.Name)
		}
		number, ok := toFloat(values[0])
		if !ok {
			return nil, errorsForType(identifier.Name)
		}
		precision := float64(0)
		if len(values) == 2 {
			precision, ok = toFloat(values[1])
			if !ok {
				return nil, errorsForType(identifier.Name)
			}
		}
		factor := math.Pow10(int(precision))
		return math.Round(number*factor) / factor, nil
	default:
		return nil, fmt.Errorf("不支持的计算函数: %s", identifier.Name)
	}
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	if number, ok := toFloat(value); ok {
		return number != 0
	}
	if stringValue, ok := value.(string); ok {
		return stringValue != ""
	}
	return true
}

func numericEqual(left, right any) bool {
	leftNumber, leftOK := toFloat(left)
	rightNumber, rightOK := toFloat(right)
	return leftOK && rightOK && leftNumber == rightNumber
}

func errorsForOperator(operator string) error {
	return fmt.Errorf("不支持或无法计算的运算: %s", operator)
}
func errorsForArgumentCount(name string) error {
	return fmt.Errorf("函数 %s 参数数量无效", name)
}
func errorsForType(name string) error    { return fmt.Errorf("函数 %s 参数类型无效", name) }
func sourceForNode(node ast.Expr) string { return fmt.Sprintf("%T", node) }
